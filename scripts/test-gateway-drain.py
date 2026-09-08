#!/usr/bin/env python3
"""Exercise a real gateway process through a connection-preserving drain."""

import argparse
import json
import os
import queue
import signal
import socket
import subprocess
import sys
import threading
import time


def wait_for_log(lines, predicate, description, timeout=15):
    deadline = time.monotonic() + timeout
    seen = []
    while time.monotonic() < deadline:
        try:
            line = lines.get(timeout=min(0.25, deadline - time.monotonic()))
        except queue.Empty:
            continue
        seen.append(line)
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if predicate(record):
            return record
    raise RuntimeError(f"timed out waiting for {description}; recent logs: {seen[-5:]}")


def receive_until(conn, expected, timeout=5):
    conn.settimeout(0.25)
    deadline = time.monotonic() + timeout
    received = bytearray()
    while time.monotonic() < deadline:
        try:
            received.extend(conn.recv(4096))
        except socket.timeout:
            continue
        if expected in received:
            return bytes(received)
    raise RuntimeError(f"terminal did not send {expected!r}; received {bytes(received)!r}")


def connection_refused(port):
    try:
        conn = socket.create_connection(("127.0.0.1", port), timeout=1)
    except OSError:
        return True
    conn.close()
    return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", default="build/telnet-gateway")
    args = parser.parse_args()
    ansi_port = int(os.environ.get("TELNET_PORT", "2323"))
    petscii_port = int(os.environ.get("PETSCII_PORT", "6464"))

    process = subprocess.Popen(
        [args.binary],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
    )
    lines = queue.Queue()

    def collect_logs():
        for line in process.stdout:
            lines.put(line.rstrip())

    threading.Thread(target=collect_logs, daemon=True).start()
    existing = None
    try:
        for _ in range(2):
            wait_for_log(
                lines,
                lambda record: record.get("msg") == "Telnet server started",
                "both terminal listeners",
            )

        existing = socket.create_connection(("127.0.0.1", ansi_port), timeout=2)
        receive_until(existing, b"Enter your username:")

        process.send_signal(signal.SIGUSR1)
        for _ in range(2):
            wait_for_log(
                lines,
                lambda record: record.get("msg") == "Telnet server draining",
                "both terminal listeners to drain",
            )

        existing.sendall(b"draincheck\r\n")
        receive_until(existing, b"Once you've authenticated")

        for port in (ansi_port, petscii_port):
            if not connection_refused(port):
                raise RuntimeError(f"drained listener on port {port} accepted a new connection")

        print("PASS: existing terminal survived SIGUSR1 and both listeners refused new clients")
    finally:
        if existing is not None:
            existing.close()
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if process.returncode != 0:
            print(f"gateway exited with status {process.returncode}", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
