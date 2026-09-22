import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import pull
import status


class PullTests(unittest.TestCase):
    def release(self):
        return {'tag_name': 'v0.1.1', 'draft': False, 'prerelease': False, 'assets': [
            {'name': 'rck-linux-arm64.tar.gz', 'size': 100, 'digest': 'sha256:' + 'a' * 64,
             'browser_download_url': 'https://github.com/mchineboy/sid64-quest/releases/download/v0.1.1/rck-linux-arm64.tar.gz'},
            {'name': 'rck-linux-arm64.tar.gz.sha256'}]}

    def test_only_ready_stable_releases_at_or_above_bootstrap_floor(self):
        release = self.release()
        self.assertIsNotNone(pull.candidate(release, 'v0.1.1'))
        self.assertIsNone(pull.candidate(release, 'v0.1.2'))
        release['assets'].pop()
        self.assertIsNone(pull.candidate(release, 'v0.1.1'))
        for flag in ('draft', 'prerelease'):
            release = self.release()
            release[flag] = True
            with self.assertRaises(ValueError):
                pull.candidate(release, 'v0.1.1')

    def test_asset_origin_size_and_digest_required(self):
        for change in ({'digest': ''}, {'size': 100 * 1024**2}, {'browser_download_url': 'https://example.test/evil'}):
            release = self.release()
            release['assets'][0].update(change)
            with self.assertRaises(ValueError):
                pull.candidate(release, 'v0.1.1')

    def test_tag_must_be_on_main(self):
        for comparison in ('behind', 'diverged'):
            with patch.object(pull, 'get_json', side_effect=[{'object': {'type': 'commit', 'sha': 'a' * 40}}, {'status': comparison}]):
                with self.assertRaises(ValueError):
                    pull.resolve_commit('v0.1.1')
        with patch.object(pull, 'get_json', side_effect=[{'object': {'type': 'commit', 'sha': 'a' * 40}}, {'status': 'ahead'}]):
            self.assertEqual(pull.resolve_commit('v0.1.1'), 'a' * 40)

    def test_newer_build_does_not_hide_ready_release(self):
        ready = self.release()
        pending = {**self.release(), 'tag_name': 'v0.1.2', 'assets': []}
        prerelease = {**self.release(), 'tag_name': 'v0.2.0', 'prerelease': True}
        chosen, _ = pull.ready_release([pending, prerelease, ready], 'v0.1.1')
        self.assertEqual(chosen['tag_name'], 'v0.1.1')
        self.assertIsNone(pull.ready_release([pending, prerelease], 'v0.1.1'))

    def test_status_never_exposes_extra_fields(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state.json'
            self.assertEqual(status.public_status(path), {'status': 'waiting'})
            path.write_text(json.dumps({'version': 'v0.1.1', 'status': 'complete', 'secret': 'hidden', 'env': 'hidden'}))
            self.assertEqual(status.public_status(path), {'version': 'v0.1.1', 'status': 'complete'})


if __name__ == '__main__':
    unittest.main()
