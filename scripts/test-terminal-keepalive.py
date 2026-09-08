#!/usr/bin/env python3
"""Hold both public terminal sockets idle, then check they still answer input.

No accounts or gameplay changes. Client TCP keep-alive is deliberately disabled;
inspect the proxy's socket timers separately to verify server-side probes.
"""

import argparse
import select
import socket
import time
from contextlib import ExitStack


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--seconds", type=float, default=75)
    args = parser.parse_args()
    if args.seconds <= 0:
        parser.error("--seconds must be positive")

    with ExitStack() as stack:
        sockets = [
            stack.enter_context(socket.create_connection((args.host, port), timeout=10))
            for port in (2323, 6464)
        ]
        for conn in sockets:
            conn.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 0)
        received = {conn: 0 for conn in sockets}
        deadline = time.monotonic() + args.seconds
        print(f"Holding both sockets idle for {args.seconds:g}s", flush=True)
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            ready, _, _ = select.select(sockets, [], [], min(5, remaining))
            for conn in ready:
                data = conn.recv(65536)
                if not data:
                    raise RuntimeError(f"EOF on port {conn.getpeername()[1]}")
                received[conn] += len(data)
        for conn in sockets:
            port = conn.getpeername()[1]
            if not received[conn]:
                raise RuntimeError(f"No welcome data on port {port}")
            conn.sendall(b"\r\n")
            conn.settimeout(15)
            if not conn.recv(65536):
                raise RuntimeError(f"EOF after input on port {port}")
            print(f"PASS {port}: same socket survived idle and answered input", flush=True)


if __name__ == "__main__":
    main()
