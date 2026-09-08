# Quick start

The short version:

```bash
docker compose -f docker-compose.simple.yml up -d
make run-auth-service       # terminal 1
make run-telnet-gateway     # terminal 2
telnet localhost 2323       # terminal 3
```

For a Commodore/PETSCII terminal, use port `6464` instead of `2323`.

Log in as `admin` with password `admin123`, use the one-time browser link printed by telnet, then select character `1` when the terminal advances automatically.

This is only for a fresh local development database. The default account and passwords must not be used in a public deployment.

For prerequisites, the exact login flow, available commands, and the current project boundaries, see [README.md](README.md).
