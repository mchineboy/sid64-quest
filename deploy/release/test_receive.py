import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import receive


class ReceiverTests(unittest.TestCase):
    def setUp(self):
        self.binary = b'\x7fELF\x02\x01' + b'\0' * 12 + b'\xb7\x00' + b'payload'
        self.manifest = {'format': 1, 'repository': 'mchineboy/sid64-quest',
                         'version': 'v1.2.3', 'commit': 'a' * 40,
                         'files': {name: hashlib.sha256(self.binary).hexdigest() for name in receive.BINARIES}}

    def archive(self, extra=None, omit=None, corrupt=False):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode='w:gz') as archive:
            for name in sorted(receive.BINARIES | {'manifest.json'}):
                if name == omit:
                    continue
                data = json.dumps(self.manifest).encode() if name == 'manifest.json' else self.binary
                if corrupt and name == 'game-core':
                    data += b'corrupt'
                info = tarfile.TarInfo(name)
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            if extra:
                archive.addfile(extra, io.BytesIO(b'x' * extra.size))
        stream.seek(0)
        return stream

    def accept(self, stream):
        with tempfile.TemporaryDirectory() as directory:
            return receive.receive(stream, Path(directory))

    def test_complete_release(self):
        self.assertEqual(self.accept(self.archive()), self.manifest)

    def test_rejects_path_traversal_links_duplicates_and_oversize(self):
        for name, kind, size in [('../outside', tarfile.REGTYPE, 1),
                                 ('game-core', tarfile.SYMTYPE, 0),
                                 ('game-core', tarfile.REGTYPE, 1),
                                 ('manifest.json', tarfile.REGTYPE, 65537)]:
            with self.subTest(name=name, kind=kind):
                entry = tarfile.TarInfo(name)
                entry.type, entry.size, entry.linkname = kind, size, '/etc/passwd'
                with self.assertRaises(ValueError):
                    self.accept(self.archive(extra=entry))

    def test_rejects_incomplete_or_tampered_release(self):
        for stream in (self.archive(omit='auth-service'), self.archive(corrupt=True)):
            with self.assertRaises(ValueError):
                self.accept(stream)

    def test_rejects_wrong_architecture(self):
        self.binary = self.binary[:18] + b'\x3e\x00' + self.binary[20:]
        self.manifest['files'] = {name: hashlib.sha256(self.binary).hexdigest() for name in receive.BINARIES}
        with self.assertRaises(ValueError):
            self.accept(self.archive())

    def test_stable_tags_only(self):
        for value in ('v1.2.3-rc1', '../evil', 'v01.2.3', 'main'):
            self.manifest['version'] = value
            with self.assertRaises(ValueError):
                self.accept(self.archive())

    def test_versions_are_monotonic_and_immutable(self):
        old = dict(self.manifest)
        self.assertFalse(receive.check_version(self.manifest, old))
        for changes in ({'version': 'v1.2.2'}, {'commit': 'b' * 40}, {'files': {}}):
            with self.assertRaises(ValueError):
                receive.check_version({**self.manifest, **changes}, old)
        self.assertTrue(receive.check_version({**self.manifest, 'version': 'v1.3.0'}, old))

    def test_env_pin_preserves_secrets_and_permissions(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(receive, 'ROOT', Path(directory)):
            path = Path(directory) / '.env'
            path.write_text('AUTH_SECRET_KEY=keep-this\nCORE_BLUE_IMAGE=rck:old\n')
            receive.pin('CORE_BLUE_IMAGE', 'rck:v1.2.3')
            self.assertEqual(path.read_text(), 'AUTH_SECRET_KEY=keep-this\nCORE_BLUE_IMAGE=rck:v1.2.3\n')
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(ValueError):
                receive.pin('MISSING_IMAGE', 'rck:v1.2.3')

    def test_failed_candidate_never_switches_active_core(self):
        with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryDirectory() as staged:
            root = Path(directory)
            (root / 'releases').mkdir()
            (root / '.env').write_text('CORE_GREEN_IMAGE=rck:old\n')
            (root / 'Dockerfile').write_text('FROM scratch\n')
            calls = []
            def compose(*args, **kwargs):
                calls.append(args)
                raise RuntimeError('Candidate not ready')
            with patch.object(receive, 'ROOT', root), patch.object(receive, 'INSTALLED', root), \
                 patch.object(receive, 'run'), patch.object(receive, 'backup', return_value='backups/test.dump'), \
                 patch.object(receive, 'compose', side_effect=compose):
                with self.assertRaises(RuntimeError):
                    receive.deploy(Path(staged), self.manifest, 'http://core-blue:8082')
            self.assertEqual(len(calls), 1)
            self.assertNotIn('core-switch', str(calls))
            state = json.loads((root / 'releases/v1.2.3/deployment.json').read_text())
            self.assertEqual(state['status'], 'failed')
            self.assertEqual(state['status_at_failure'], 'preparing')


if __name__ == '__main__':
    unittest.main()
