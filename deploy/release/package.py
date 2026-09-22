#!/usr/bin/env python3
"""Package only ARM64 release binaries and a checksummed deployment manifest."""
import hashlib
import gzip
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile

BINARIES = ("auth-service", "telnet-gateway", "rck-admin", "game-core", "terminal-edge", "core-switch")


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    version, output = sys.argv[1:]
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise SystemExit("Use a stable version tag: vMAJOR.MINOR.PATCH")
    if subprocess.check_output(["git", "status", "--porcelain"]).strip():
        raise SystemExit("Release packaging requires a clean checkout")
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    binary_dir = Path("build/release")
    binary_dir.mkdir(parents=True, exist_ok=True)
    subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-o", str(binary_dir) + "/", "./cmd/..."],
                   env={**os.environ, "GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0"}, check=True)
    configs = {"compose.yml": "deploy/pi/compose.yml", "compose.edge.yml": "deploy/pi/compose.edge.yml",
               "Caddyfile": "deploy/ec2/Caddyfile", "Dockerfile": "deploy/pi/Dockerfile"}
    manifest = {"format": 1, "repository": "mchineboy/sid64-quest", "version": version, "commit": commit,
                "files": {name: digest(binary_dir / name) for name in BINARIES},
                "configurations": {name: digest(path) for name, path in configs.items()},
                "receiver_sha256": digest("deploy/release/receive.py"),
                "migrations": {p.name: digest(p) for p in sorted(Path("internal/database/migrations").glob("*.sql"))}}
    (binary_dir / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    # Identical commit/version produces identical bytes, including workflow reruns.
    with open(output, "wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w") as archive:
            for name in (*BINARIES, "manifest.json"):
                path = binary_dir / name
                info = tarfile.TarInfo(name)
                info.size = path.stat().st_size
                info.mode = 0o644 if name == "manifest.json" else 0o755
                with path.open("rb") as source:
                    archive.addfile(info, source)
    Path(output + ".sha256").write_text(digest(output) + "  " + Path(output).name + "\n")
    print(f"Packaged {version} at {commit}")


if __name__ == "__main__":
    main()
