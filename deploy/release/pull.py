#!/usr/bin/env python3
"""Outbound-only, fixed-function release watcher; no GitHub credentials required."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

from receive import ROOT, INSTALLED, VERSION, atomic_write, require

REPO = 'mchineboy/sid64-quest'
API = 'https://api.github.com/repos/' + REPO
STATUS = ROOT / 'release-status.json'


def get_json(path):
    request = urllib.request.Request(API + path, headers={'Accept': 'application/vnd.github+json', 'User-Agent': 'sid64-release-agent'})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def version_tuple(value):
    match = VERSION.fullmatch(value)
    require(match, 'Invalid stable version')
    return tuple(map(int, match.groups()))


def candidate(release, minimum):
    require(not release['draft'] and not release['prerelease'], 'Not a stable release')
    version = release['tag_name']
    if version_tuple(version) < version_tuple(minimum):
        return None
    assets = {asset['name']: asset for asset in release['assets']}
    if not {'rck-linux-arm64.tar.gz', 'rck-linux-arm64.tar.gz.sha256'} <= assets.keys():
        return None  # The build/test workflow has not published the assets yet.
    asset = assets['rck-linux-arm64.tar.gz']
    require(asset.get('digest', '').startswith('sha256:'), 'GitHub asset digest unavailable')
    require(0 < asset['size'] <= 96 * 1024**2, 'Invalid asset size')
    expected = f'https://github.com/{REPO}/releases/download/{version}/rck-linux-arm64.tar.gz'
    require(asset['browser_download_url'] == expected, 'Unexpected release download URL')
    return asset


def resolve_commit(version):
    obj = get_json('/git/ref/tags/' + version)['object']
    for _ in range(4):
        if obj['type'] == 'commit':
            break
        require(obj['type'] == 'tag', 'Unexpected tag object')
        obj = get_json('/git/tags/' + obj['sha'])['object']
    require(obj['type'] == 'commit', 'Tag nesting exceeds limit')
    commit = obj['sha']
    comparison = get_json('/compare/' + commit + '...main')
    require(comparison['status'] in ('ahead', 'identical'), 'Release commit is not on main')
    return commit


def ready_release(releases, minimum):
    stable = [r for r in releases if not r['draft'] and not r['prerelease'] and VERSION.fullmatch(r['tag_name'])]
    for release in sorted(stable, key=lambda r: version_tuple(r['tag_name']), reverse=True):
        asset = candidate(release, minimum)
        if asset:
            return release, asset
    return None


def download(asset, path):
    request = urllib.request.Request(asset['browser_download_url'], headers={'User-Agent': 'sid64-release-agent'})
    digest, total = hashlib.sha256(), 0
    with urllib.request.urlopen(request, timeout=60) as response, path.open('wb') as output:
        while chunk := response.read(1024 * 1024):
            total += len(chunk)
            require(total <= asset['size'], 'Asset exceeds declared size')
            digest.update(chunk)
            output.write(chunk)
    require(total == asset['size'] and 'sha256:' + digest.hexdigest() == asset['digest'], 'GitHub asset digest mismatch')


def main():
    os.umask(0o077)
    minimum = (INSTALLED / 'min-version').read_text().strip()
    # A newer release still building must not hide a ready release whose Actions
    # job holds production concurrency while waiting for this agent.
    ready = ready_release(get_json('/releases?per_page=20'), minimum)
    if ready is None:
        return
    release, asset = ready
    version = release['tag_name']
    previous = json.loads(STATUS.read_text()) if STATUS.exists() else {}
    if previous.get('version') and version_tuple(previous['version']) >= version_tuple(version):
        return  # Completed/failed/interrupted attempts require a newer version or operator repair.
    commit = resolve_commit(version)
    state = {'version': version, 'commit': commit, 'status': 'downloading', 'updated_at': time.time()}
    def save(status):
        state.update(status=status, updated_at=time.time())
        atomic_write(STATUS, json.dumps(state) + '\n')
    save('downloading')
    try:
        with tempfile.TemporaryDirectory(prefix='.pull-', dir=ROOT) as temporary:
            archive = Path(temporary) / 'release.tar.gz'
            download(asset, archive)
            # Validate archive and cross-check manifest with the GitHub tag before
            # invoking the same receiver used by operator verification.
            from receive import receive
            staging = Path(temporary) / 'checked'
            staging.mkdir()
            with archive.open('rb') as stream:
                manifest = receive(stream, staging)
            require(manifest['commit'] == commit and manifest['version'] == version, 'Artifact does not match tag')
            save('deploying')
            with archive.open('rb') as stream:
                subprocess.run([str(INSTALLED / 'receive.py')], stdin=stream,
                               env={**os.environ, 'SSH_ORIGINAL_COMMAND': 'deploy'}, check=True, timeout=1800)
            save('complete')
    except Exception:
        save('failed')
        raise


if __name__ == '__main__':
    main()
