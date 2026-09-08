#!/usr/bin/env python3
"""Pin the initial edge/core release without exporting or replacing Pi secrets."""
import argparse
import os
from pathlib import Path
import re
import secrets
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--release", required=True)
    parser.add_argument("--directory", default="/srv/rck")
    args = parser.parse_args()
    if not re.fullmatch(r"rck:[a-zA-Z0-9_.-]+", args.release):
        parser.error("release must be an explicit rck image tag")
    root = Path(args.directory)
    env = root / ".env"
    original = env.read_text()
    if (root / "edge-control" / "target").exists():
        raise SystemExit("Already migrated; update only the inactive core slot.")
    # Retain the exact old environment for rollback, once only.
    backup = root / ".env.pre-edge"
    if not backup.exists():
        fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write(original)
    updates = {
        "CORE_TOKEN": secrets.token_hex(32),
        "EDGE_IMAGE": args.release,
        "CORE_BLUE_IMAGE": args.release,
        "CORE_GREEN_IMAGE": args.release,
        "AUTH_IMAGE": args.release,
        "COMPOSE_FILE": "compose.edge.yml",
    }
    retained = [line for line in original.splitlines()
                if line.split("=", 1)[0] not in updates]
    text = "\n".join(retained + [f"{key}={value}" for key, value in updates.items()]) + "\n"
    fd, temporary = tempfile.mkstemp(prefix=".env.edge-", dir=root)
    try:
        with os.fdopen(fd, "w") as stream:
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, env)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    print(f"Pinned edge, both cores and auth to {args.release}; original environment saved privately.")


if __name__ == "__main__":
    main()
