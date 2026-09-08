# SID64 Quest alpha operations

Host: `ssh tyler.hardison@symptom-pi` (Debian ARM64, hostname modeburner). Deployment: `/srv/rck`. One gateway process owns presence for both terminal modes; do not scale gateway replicas.

Public web: https://sid64.quest/account
Public ANSI: `sid64.quest:2323`; public PETSCII: `sid64.quest:6464`.
The private `symptom-pi` endpoints remain available for operator diagnostics over the tailnet. The existing symptom tracker retains ports 80/443 and 3000. MUD HTTP is loopback port 8081; databases have no published host ports.

## Deploy and inspect

From the project on the Mac, source the development environment and run `go test -race -count=1 ./...` and `go vet ./...`. Then run `scripts/deploy-pi.sh`. It builds ARM64 binaries, takes a pre-deploy backup, retains the current image as `rck:previous`, and waits for healthy containers. Deployment interrupts terminal sessions. It never copies the local `.env`.

On the Pi:

```sh
cd /srv/rck
docker compose ps
docker compose logs --tail=100 auth gateway
sudo systemctl status rck.service rck-backup.timer
curl --fail http://127.0.0.1:8081/ready
```

`rck.service` waits for the Tailscale address before starting the stack at boot. `sudo systemctl restart rck.service` restarts only the MUD stack. Containers also have restart policies. `/ready` checks PostgreSQL and Redis; gateway health checks both listeners. Logs rotate for application containers. Check disk use periodically, including database logs and backups.

If a code release fails and its migrations are compatible with the previous code:

```sh
cd /srv/rck
docker tag rck:previous rck:alpha
docker compose up -d --force-recreate --wait
```

Database migrations are numbered, embedded, checksum-checked and transactional. Never edit an applied migration. A code rollback does not undo a migration. Review schema compatibility before rolling back.

## Accounts

On the Pi in `/srv/rck`:

```sh
docker compose exec -T auth /app/rck-admin list
docker compose exec -T auth /app/rck-admin disable USERNAME
docker compose exec -T auth /app/rck-admin enable USERNAME
```

For a password reset, use Bash and a hidden prompt, keeping the password out of command arguments and history:

```bash
read -r -s -p 'New password: ' rck_password
printf '\n'
printf '%s\n' "$rck_password" | docker compose exec -T auth /app/rck-admin reset-password USERNAME
unset rck_password
```

Passwords must be 8–72 bytes. Resetting a password invalidates browser sessions on their next authenticated request. Disabling an account also rejects its next terminal command; password resets alone do not disconnect an existing terminal session. CLI actions print an outcome; retain operator records where needed. There is no comprehensive durable moderation audit system yet.

## Backups and recovery

`rck-backup.timer` runs daily around 04:15 Pi time. `/srv/rck/backup.sh` uses PostgreSQL custom dumps, validates the archive index, flushes and atomically renames the completed dump, and writes SHA256 checksums. Pi retention is 30 days. This reduces partial-file risk; it is not a guarantee against media or power failure.

On the Mac, `org.raceconditionkingdom.backups` runs `scripts/pull-pi-backups.sh` every six hours and at load. The Mac must be awake and connected. Copies live in ignored `backups/pi/`, with checksum verification and 60-day retention. Logs are `build/vice/backup-pull.log` and `build/vice/backup-pull-error.log`. Check `launchctl print gui/$(id -u)/org.raceconditionkingdom.backups` for the last exit code. Backups contain private account data; keep these directories private.

Manual backup and nondestructive restore rehearsal on the Pi:

```sh
cd /srv/rck
./backup.sh
./restore-check.sh backups/TIMESTAMP.dump
```

The rehearsal restores into a disposable database and deletes only that scratch database. It checks migration, room and character counts. PostgreSQL backups preserve accounts/world/inventory, but do not include Redis sessions, deployment secrets or the systemd configuration. Keep the Pi `.env` in a separate secure operator backup.

For actual disaster recovery, verify the selected checksum and rehearse its restore first. Stop auth and gateway, preserve a dump of the current database if possible, then replace the live database using the selected archive and matching application version. This deliberately destructive procedure needs an operator to choose the recovery point. Start services and validate login and saved state. Users may need to sign in and pair again. A controlled reboot was issued on 2026-09-08 after backup `20260908T161453Z.dump`. After subsequent boot attempts, the inspected boot (`db8b166d-94d1-4e30-8c14-3be2d88c2283`) took 8 minutes 38.660 seconds. The kernel logged `mmc0: Card stuck being busy! __mmc_poll_for_busy` at 296 and 477 seconds. NetworkManager's online wait also failed; its exact cause remains undetermined. Storage stalls are the strongest evidence for the delay, but card wear versus controller/firmware behavior has not been isolated. Root storage is a 119.4 GiB SD card, 13% full; `vcgencmd get_throttled` returned `0x0` for this boot.

All four MUD containers eventually became healthy automatically; rck.service and the backup timer are active. MUD HTTPS returned 200 and the existing tracker returned its expected 303 redirect. The database matches the pre-reboot state (zero accounts/characters, five rooms); the character fingerprint matches. This verifies recovery of the empty alpha world, not populated-world durability through a power cut. The pre-reboot archive was copied to the Mac and its checksum passed. No boot settings were changed during diagnosis. Migrate storage before further release testing; back up the tracker and deployment secrets separately before migration.

## Secrets and exposure

The Pi has fresh random database, Redis and auth secrets in mode-600 `/srv/rck/.env`; no seeded admin exists. `deploy/pi/bootstrap-env.py` creates the initial environment and preserves an existing one. `.env` is ignored and removed from tracking, but old development credentials remain in Git history and must never be reused. History has not been rewritten.

Development setup generates new secrets for a new `.env`. Optional dashboard profiles require explicit PGADMIN_PASSWORD, MONGO_EXPRESS_PASSWORD and GRAFANA_PASSWORD values. The Pi does not run MongoDB or these dashboards.

The alpha uses Tailscale Serve, not Funnel. Preserve other Serve routes. To disable only the MUD web route, use `sudo tailscale serve --https=8443 off`; do not reset the whole Serve configuration. Telnet traffic is protected by the tailnet tunnel, while browser credentials use HTTPS. Before public hosting, revisit transport protection, abuse limits, password recovery, monitoring and moderation.

## Release checks

`scripts/alpha-smoke.py` exercises real HTTPS signup/pairing and both live terminal ports, shared presence/chat, duplicate login prevention, reconnect and saved location. It creates disposable accounts listed in `build/pi/smoke-users.json`; delete only those exact test accounts after checking results. Automated socket tests cannot establish CCGMS visual quality or real-hardware behavior. Use [the tester guide](ALPHA-TESTERS.md) for those checks.

## Hardware access update

Terminal listeners now use `MUD_BIND_IP=0.0.0.0` on the Pi for LAN and tailnet access. HTTPS account management remains tailnet-only. No router forwarding was added. For the Commodore on the Mac's Internet Sharing subnet, use the SSH relay described in [VICE testing](VICE-TESTING.md): `192.168.2.1:26464`. This reaches the deployed Pi, not the older local development server.

## Public endpoint activation — 2026-09-08

`sid64.quest` resolves to EC2 at `52.34.32.84`. Caddy is enabled with public HTTPS; HAProxy forwards ports 2323 and 6464 to the Pi. AUTH_BASE_URL is now `https://sid64.quest`. Public users and QR-scanning phones no longer need Tailscale. Earlier private-only instructions describe the previous deployment. The existing Pi ts.net routes are preserved.
