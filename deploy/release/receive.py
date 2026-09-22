#!/usr/bin/env python3
"""Fixed SSH command installed by an operator; accepts a release archive on stdin.

No repository scripts, Compose files, shell commands, or environments from the
archive are executed. Changes to infrastructure require a separate installation.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request

ROOT = Path('/srv/rck')
INSTALLED = Path('/usr/local/lib/rck-release')
BINARIES = {'auth-service', 'telnet-gateway', 'rck-admin', 'game-core', 'terminal-edge', 'core-switch'}
VERSION = re.compile(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)')
SHA = re.compile(r'[0-9a-f]{64}')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def receive(stream, directory):
    seen, total = set(), 0
    with tarfile.open(fileobj=stream, mode='r|gz') as archive:
        for member in archive:
            require(member.name in BINARIES | {'manifest.json'}, 'Unexpected archive entry')
            require(member.name not in seen and member.isfile(), 'Duplicate or non-regular archive entry')
            limit = 65536 if member.name == 'manifest.json' else 64 * 1024 * 1024
            require(0 < member.size <= limit, 'Invalid file size')
            total += member.size
            require(total <= 256 * 1024 * 1024, 'Release exceeds size limit')
            path = directory / member.name
            with archive.extractfile(member) as source, path.open('xb') as output:
                shutil.copyfileobj(source, output)
            path.chmod(0o600 if member.name == 'manifest.json' else 0o755)
            seen.add(member.name)
    require(seen == BINARIES | {'manifest.json'}, 'Incomplete release')
    manifest = json.loads((directory / 'manifest.json').read_text())
    require(manifest.get('format') == 1 and manifest.get('repository') == 'mchineboy/sid64-quest', 'Wrong release format/repository')
    require(VERSION.fullmatch(manifest.get('version', '')), 'Invalid stable version')
    require(re.fullmatch(r'[0-9a-f]{40}', manifest.get('commit', '')), 'Invalid commit')
    require(set(manifest.get('files', {})) == BINARIES, 'Wrong binary set')
    for name in BINARIES:
        require(digest(directory / name) == manifest['files'][name], 'Binary checksum mismatch')
        with (directory / name).open('rb') as binary:
            header = binary.read(20)
        require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\xb7\x00', 'Expected Linux ARM64 ELF')
    return manifest


def run(*args, capture=False, timeout=600):
    result = subprocess.run(args, cwd=ROOT, check=True, timeout=timeout,
                            stdout=subprocess.PIPE if capture else None, text=True)
    return result.stdout.strip() if capture else None


def compose(*args, **kwargs):
    return run('docker', 'compose', '-f', str(ROOT / 'compose.edge.yml'), *args, **kwargs)


def atomic_write(path, data, mode=0o600):
    fd, name = tempfile.mkstemp(prefix='.' + path.name, dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as output:
            os.fchmod(output.fileno(), mode)
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def pin(key, image):
    path = ROOT / '.env'
    lines = path.read_text().splitlines()
    matches = [i for i, line in enumerate(lines) if line.startswith(key + '=')]
    require(len(matches) == 1, 'Missing or duplicate deployment image pin')
    lines[matches[0]] = key + '=' + image
    atomic_write(path, '\n'.join(lines) + '\n')


def check_version(manifest, previous):
    if previous:
        old = tuple(map(int, VERSION.fullmatch(previous['version']).groups()))
        new = tuple(map(int, VERSION.fullmatch(manifest['version']).groups()))
        require(new >= old, 'Automatic version downgrade refused; publish a newer corrective release')
        if new == old:
            require(manifest == previous, 'Published version is immutable')
            return False
    return True


def preflight(manifest):
    expected = {'compose.yml': ROOT / 'compose.yml', 'compose.edge.yml': ROOT / 'compose.edge.yml',
                'Caddyfile': INSTALLED / 'Caddyfile', 'Dockerfile': INSTALLED / 'Dockerfile'}
    require(set(manifest.get('configurations', {})) == set(expected), 'Missing infrastructure fingerprints')
    for name, path in expected.items():
        require(digest(path) == manifest['configurations'][name], 'Infrastructure change requires operator installation: ' + name)
    require(digest(INSTALLED / 'receive.py') == manifest.get('receiver_sha256'), 'Deployment receiver requires operator update')
    migrations = manifest.get('migrations', {})
    require(migrations and all(re.fullmatch(r'[0-9]{3}_[a-z0-9_]+\.sql', k) and SHA.fullmatch(v)
                               for k, v in migrations.items()), 'Invalid migration manifest')
    saved = compose('exec', '-T', 'postgres', 'psql', '-U', 'mud_user', '-d', 'race_condition_kingdom',
                    '-At', '-F', '|', '-c', 'SELECT version,checksum FROM schema_migrations ORDER BY version', capture=True)
    for row in saved.splitlines():
        name, checksum = row.split('|')
        require(migrations.get(name) == checksum, 'Applied migration changed or missing: ' + name)
    require(shutil.disk_usage(ROOT).free > 2 * 1024**3, 'Less than 2 GiB free space')
    target = compose('exec', '-T', 'edge', 'cat', '/control/target', capture=True)
    require(target in ('http://core-blue:8082', 'http://core-green:8082'), 'Unrecognized active core')
    # A release must not start from an already unhealthy stack.
    healthy()
    return target


def healthy():
    for service in ('postgres', 'redis', 'auth', 'edge', 'core-blue', 'core-green'):
        cid = compose('ps', '-q', service, capture=True)
        require(cid, 'Missing service: ' + service)
        status = run('docker', 'inspect', '--format', '{{.State.Health.Status}}', cid, capture=True)
        require(status == 'healthy', 'Unhealthy service: ' + service)
    for path in ('ready', 'login'):
        with urllib.request.urlopen('https://sid64.quest/' + path, timeout=20) as response:
            require(response.status == 200, 'Public HTTP probe failed')
    for port in (2323, 6464):
        with socket.create_connection(('127.0.0.1', port), timeout=10) as connection:
            connection.settimeout(10)
            require(connection.recv(512), 'Terminal returned no data')


def backup():
    output = run(str(ROOT / 'backup.sh'), capture=True)
    match = re.fullmatch(r'Backup complete: ([0-9]{8}T[0-9]{6}Z\.dump)', output)
    require(match, 'Backup did not complete (possibly another backup holds the lock)')
    archive = 'backups/' + match.group(1)
    run('sha256sum', '-c', archive + '.sha256')
    run(str(ROOT / 'restore-check.sh'), archive)
    return archive


def deploy(directory, manifest, target):
    version = manifest['version']
    image = 'rck:' + version
    record = ROOT / 'releases' / version
    require(not record.exists(), 'Version already attempted; inspect its record before retrying')
    record.mkdir(mode=0o700)
    shutil.copy2(ROOT / '.env', record / 'env.before')
    (record / 'target.before').write_text(target + '\n')
    state = {'version': version, 'commit': manifest['commit'], 'status': 'preparing', 'started_at': time.time()}
    def save():
        atomic_write(record / 'deployment.json', json.dumps(state, indent=2) + '\n')
    save()
    try:
        shutil.copytree(directory, record / 'image')
        shutil.copy2(INSTALLED / 'Dockerfile', record / 'image' / 'Dockerfile')
        run('docker', 'build', '-t', image, str(record / 'image'))
        state['backup_before'] = backup()
        save()
        active = target.split('//')[1].split(':')[0]
        candidate = 'core-green' if active == 'core-blue' else 'core-blue'
        pin(candidate.upper().replace('-', '_') + '_IMAGE', image)
        compose('up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', candidate)
        state['status'] = 'candidate-ready'
        save()
        compose('exec', '-T', candidate, '/app/core-switch', '-target', 'http://' + candidate + ':8082', '-file', '/control/target')
        state['status'] = 'switched'
        save()
        # Do not leave an old core running with potentially obsolete auth checks.
        compose('stop', active)
        pin('AUTH_IMAGE', image)
        compose('up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', 'auth')
        old_edge = compose('exec', '-T', 'edge', 'sha256sum', '/app/terminal-edge', capture=True).split()[0]
        state['edge_restarted'] = old_edge != manifest['files']['terminal-edge']
        if state['edge_restarted']:
            print('Edge binary changed: replacing the edge disconnects terminal sockets.', flush=True)
            pin('EDGE_IMAGE', image)
            compose('up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', 'edge')
        pin(active.upper().replace('-', '_') + '_IMAGE', image)
        compose('up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', active)
        admin_dir = ROOT / 'admin' / version
        admin_dir.mkdir()
        shutil.copy2(directory / 'rck-admin', admin_dir / 'rck-admin')
        link = ROOT / 'admin' / ('.current-' + version)
        link.symlink_to(version)
        link.replace(ROOT / 'admin' / 'current')
        healthy()
        state['backup_after'] = backup()
        state['image_id'] = run('docker', 'image', 'inspect', '--format', '{{.Id}}', image, capture=True)
        state['status'] = 'complete'
        state['completed_at'] = time.time()
        save()
        atomic_write(ROOT / 'release-current.json', json.dumps(manifest, indent=2) + '\n')
        print(f'Deployed {version} ({manifest["commit"]}); all health checks passed.', flush=True)
    except Exception:
        state['status_at_failure'] = state['status']
        state['status'] = 'failed'
        save()
        # Automatic rollback across a schema migration can corrupt data. Retain
        # the evidence and backups; require a corrective release/operator repair.
        print('Release failed. Preserved deployment record and backups at ' + str(record), file=sys.stderr)
        raise


def main():
    os.umask(0o077)
    command = os.environ.get('SSH_ORIGINAL_COMMAND', '')
    require(command in ('verify', 'deploy'), 'Only verify and deploy are supported')
    # Bound slow/malformed uploads as well as lock waits; no inherited client env.
    os.environ.clear()
    os.environ.update(PATH='/usr/local/bin:/usr/bin:/bin', HOME=str(Path.home()), COMPOSE_PROJECT_NAME='rck',
                      DOCKER_CONFIG=str(ROOT / '.release-docker'))
    import signal
    signal.alarm(1800)
    with (ROOT / '.release.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with tempfile.TemporaryDirectory(prefix='.release-', dir=ROOT) as temporary:
            directory = Path(temporary)
            manifest = receive(sys.stdin.buffer, directory)
            current = ROOT / 'release-current.json'
            previous = json.loads(current.read_text()) if current.exists() else None
            changed = check_version(manifest, previous)
            target = preflight(manifest)
            if command == 'verify' or not changed:
                print('Release verified; no deployment changes made.', flush=True)
                return
            deploy(directory, manifest, target)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Do not include subprocess environments, HTTP cookies, or config contents.
        print('Release rejected/failed: ' + str(error), file=sys.stderr)
        sys.exit(1)
