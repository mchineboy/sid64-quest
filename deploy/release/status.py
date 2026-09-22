#!/usr/bin/env python3
"""Read-only status on loopback, routed through the existing HTTPS proxy path."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path


def public_status(path):
    data = json.loads(path.read_text()) if path.exists() else {'status': 'waiting'}
    return {key: data[key] for key in ('version', 'commit', 'status', 'updated_at') if key in data}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        data = json.dumps(public_status(Path('/srv/rck/release-status.json'))).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


if __name__ == '__main__':
    ThreadingHTTPServer(('127.0.0.1', 8091), Handler).serve_forever()
