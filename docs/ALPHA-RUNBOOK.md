# SID64 Quest alpha operations

Host: `ssh tyler.hardison@symptom-pi` (Debian ARM64, hostname modeburner).
Deployment: `/srv/rck`, using `compose.edge.yml`. One persistent edge owns both
terminal listeners; interchangeable blue/green cores share durable sessions.
See [connection-preserving core deployments](BLUE-GREEN-DEPLOYMENTS.md).

## Automated production release — 2026-09-22

Current application release: **`v0.1.1`**, commit
`06e27029bcebb5e13052d0bc0ddc3521d2270b92`, image `rck:v0.1.1`.
Auth, edge, both core slots and the operator binary are updated; **core-green is
active**. Image ID:
`sha256:52b9468b0319f01d7c94c36bc5682dec693ef2698bdf3147f49d919e418ec71a`.

Publishing the stable [GitHub Release](https://github.com/mchineboy/sid64-quest/releases/tag/v0.1.1)
triggered the [successful production workflow](https://github.com/mchineboy/sid64-quest/actions/runs/35754388411).
GitHub-hosted ARM64 tests/builds produced the artifact, and modeburner's systemd
pull agent fetched it over outbound HTTPS. No Actions runner or deployment
credential is installed in GitHub. The read-only status endpoint confirmed the
exact version/commit after successful health checks. The earlier `v0.1.0` attempt
was blocked by SSH ingress rules before any production change; those temporary
SSH deployment credentials have been revoked and removed.

The agent took and restore-tested `20260922T163222Z.dump` before deployment and
`20260922T163339Z.dump` afterward (eight migrations, 83 rooms, four characters).
All services are healthy and the original four accounts/characters remain.
The edge binary changed and was replaced during this first automated release.
The durable record is `/srv/rck/releases/v0.1.1/deployment.json`.

See [production releases](RELEASING.md) for publishing, monitoring, pausing the
agent, infrastructure changes and failure recovery. The records below describe
earlier deployments; their active-slot/image statements are historical.

## Security release — 2026-09-22

Auth, edge, both core slots, and the operator binary now use
`rck:security-20260922-d9721f1`; **core-blue is active**. This release was built
with Go 1.26.8 from the reviewed working tree based on
`d9721f1d5459a94b931ee3a3039e0be12d327983`, including uncommitted security fixes
and existing operator changes. It is not a clean-commit release. Docker image:
`sha256:833b8b9a3371a7ff79c87db603785694985d4c6bab93c7bf8728025e174a7435`.
Artifacts, binary checksums, manifest, prior Compose files, protected prior
environment, and prior target/operator links are preserved under
`/srv/rck/releases/security-20260922-d9721f1/`.

Migration 008 adds credential revocation. Public Caddy supplies authenticated
client-IP assertions using a distinct protected secret shared with auth.
Direct account access bypassing Caddy now returns 403; internal health probes
remain available. Both old cores were replaced, and the legacy gateway remains
stopped. Returning to an older binary would reintroduce the fixed vulnerabilities.

Pre-release backup `20260922T154502Z.dump` and post-release backup
`20260922T155406Z.dump` passed disposable restore checks and were copied to local
`backups/pi/` with verified SHA-256 checksums. The latter contains eight migrations,
83 rooms and four characters. All services are healthy. Public signup, both
terminal modes, idle-session revocation on password change/recovery, sibling
recovery invalidation, and separate client quotas with spoofed-header rejection
passed. The two disposable accounts and their scoped session data were removed;
the four original accounts/characters remain. No recovery email was sent.

Redis now enforces `maxmemory 268435456` and `maxmemory-policy noeviction`.
The Pi kernel lacks Docker memory-limit support, so the separate 512 MiB
container limit is not enforced. See [security remediation](SECURITY-REMEDIATION.md)
for source verification and remaining boundaries. The following records describe
earlier deployments; their active-slot/image statements are historical.

Production migrated on 2026-09-08 at 16:45 Pacific from clean `main` revision
`6d0e7d08543ef6d9c1f837a840d77ca48530b8bc`, image `rck:6d0e7d0`.
The terminal check immediately before cutover found zero established connections.
At that initial cutover both core slots, edge and auth used this pinned release,
with blue as the active target. See the newer release record below.
Public HTTPS signup/pairing, ANSI/PETSCII play and a blue→green→blue switch passed
with the same edge container and authenticated sockets. The two temporary smoke
accounts were removed, leaving the three original characters.

### Idle-event release — 2026-09-08, 17:09 Pacific

Clean, pushed `main` revision `4dc06f473c8dec5cbb30755272d12280ad38fe58`
is deployed as `rck:4dc06f4` in **core-green**, now the active target.
Blue remains on `rck:6d0e7d0` for rollback. Edge, auth, PostgreSQL and Redis
were not replaced or restarted; the edge has the same container ID/start time.
No schema migration or Redis configuration change was required.

Pre-release backup `backups/20260909T000527Z.dump` passed a disposable restore
check (four migrations, five rooms, three characters). Release artifacts and
the protected prior environment are in `/srv/rck/releases/4dc06f4/`.
Real public ANSI/PETSCII sessions authenticated before switching blue to green
and kept the same sockets. Migration notices arrived without Enter; idle
movement, admin announcements, partial input and logout notices passed.
The two disposable smoke accounts were removed after verification.
Post-release backup `backups/20260909T001006Z.dump` also passed its restore
check with the original three characters, five rooms and four migrations.

The active core supports `announce <message>` for current admins. Room-content
changes and player notices are delivered during idle polling. See the event
delivery guarantees and limits in [the deployment guide](BLUE-GREEN-DEPLOYMENTS.md).

Public web: https://sid64.quest/account
Public ANSI: `sid64.quest:2323`; public PETSCII: `sid64.quest:6464`.
The private `symptom-pi` endpoints remain available for operator diagnostics over the tailnet. The existing symptom tracker retains ports 80/443 and 3000. MUD HTTP is loopback port 8081; databases have no published host ports.

## Deploy and inspect

For normal application releases, follow [tagged production releases](RELEASING.md):
publish a stable GitHub Release and let Actions build, test, and deploy it. The
commands below remain useful for infrastructure work and incident recovery.

Build from clean, pushed `main`. Source the local development environment and run
`make test-core-restart`, `go test -race -count=1 ./...`, and `go vet ./...`.
Check applied migration checksums, take a backup, and build/upload an explicitly
tagged ARM64 release with `deploy/pi/Dockerfile`. Never copy the Mac's `.env`.
The old `scripts/deploy-pi.sh` refuses to overwrite the installed edge.

Change only the inactive core slot's image in the Pi's `.env`, start it with
`docker compose up -d --no-deps --wait core-green`, then switch with
`docker compose exec -T core-green /app/core-switch -target http://core-green:8082 -file /control/target`.
Reverse the slot names when green is active. Keep `EDGE_IMAGE` pinned through
core-only releases. Auth can be updated separately without replacing the edge.

On the Pi:

```sh
cd /srv/rck
docker compose ps
docker compose logs --tail=100 auth edge core-blue core-green
docker compose exec -T edge cat /control/target
sudo systemctl status rck.service rck-backup.timer
curl --fail http://127.0.0.1:8081/ready
```

`rck.service` explicitly selects `compose.edge.yml` and waits for Tailscale at
boot. The Pi `.env` also selects it through `COMPOSE_FILE`, including for backup
commands. `sudo systemctl restart rck.service` restarts the entire MUD stack and
**drops terminal connections**; use core-slot switching for deployments instead.
Containers have restart policies. Auth/core readiness checks PostgreSQL and
Redis; edge health checks both terminal listeners. Logs rotate. Check disk use,
including database logs and backups, periodically.

If a core release fails and blue is the retained, compatible previous version:

```sh
cd /srv/rck
docker compose up -d --no-deps --wait core-blue
docker compose exec -T core-blue /app/core-switch -target http://core-blue:8082 -file /control/target
```

Database migrations are numbered, embedded, checksum-checked and transactional. Never edit an applied migration. A code rollback does not undo a migration. Review schema compatibility before rolling back.

Initial migration rollback artifacts are retained: image `rck:pre-edge-5143cb1`,
private `.env.pre-edge`, the stopped legacy gateway container, and original
Compose/service files in `releases/6d0e7d0/`. Returning to the legacy gateway
would disconnect terminals and requires restoring its environment/service unit.
The pre-migration backup `backups/20260908T234255Z.dump` passed a disposable
restore rehearsal. Post-migration backup: `backups/20260908T234725Z.dump`.

For current gameplay presence, use in-game `who`. Operator-side checkpoint
presence can be inspected without a login (leases may lag disconnects):

```sh
docker compose exec -T postgres psql -X -U mud_user -d race_condition_kingdom -c "SELECT checkpoint->>'Username' AS username, checkpoint->'Character'->>'name' AS character, last_seen FROM terminal_sessions WHERE NOT closed AND last_seen > now()-interval '2 minutes' ORDER BY last_seen DESC"
```

## Live terminal dashboard

The independently deployed admin utility runs in a temporary container using
Compose's auth environment/network, without restarting any running service:

```sh
ssh -t tyler.hardison@symptom-pi 'cd /srv/rck && ./rck-admin top'
```

On the Pi, use `./rck-admin top --interval 5s` to refresh less often, or
`./rck-admin top --once` for a full snapshot. Redirected output automatically
uses a single snapshot. Ctrl-C exits and restores the terminal. The live view
fits as many rows as the terminal height permits; `--once` prints every row.

The view shows active session/player totals, username, character, login state,
ANSI/PETSCII mode, room and heartbeat age. Wide terminals also show an abbreviated
session ID. Heartbeats include automatic polls and **are not player idle time**.
Closed sessions are excluded; lost connections can remain visible until the
two-minute presence window expires. Connection duration, last-input time and
source IP are not currently recorded for this view. Query failures replace the
display with an error and retry, rather than displaying stale data as current.

The command selects only display fields from checkpoints, never authentication
tokens, pairing codes or pending player output. It performs no database writes.
Keep this operator command on the Pi; it uses the existing privileged database
credentials from the deployment environment.

For an admin-only release, cross-compile `./cmd/rck-admin` with
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, verify its checksum after upload, and
place it in a versioned `/srv/rck/admin/RELEASE/rck-admin` directory. Install
`deploy/pi/rck-admin.sh` as `/srv/rck/rck-admin`, then atomically point
`/srv/rck/admin/current` at the release directory. The wrapper bind-mounts this
binary into a temporary auth-image container. Existing service image pins and
containers are untouched. To roll back, repoint `admin/current` at a prior
release. This admin-only path requires no schema migration or game deployment.
The older `/app/rck-admin` inside service images remains the bundled version;
use the wrapper to access the independently released command.

### Admin dashboard release — 2026-09-21

Deployed independently at `/srv/rck/admin/top-20260921-3ed2b9e4`, with `current`
pointing to that directory and `/srv/rck/rck-admin` as the launcher. The release
contains the source snapshot and SHA256 checksums. Binary SHA256:
`3ed2b9e41a6f89957a822f7808054f5d29645869770c90d07171debbf56b28f2`.

The full race-enabled suite (including the admin database test), `go vet`, and
core process-restart integration tests passed against isolated local databases.
Production checks confirmed ANSI/PETSCII connections appear with the correct
mode and username-entry state, disappear after disconnect, and the SSH live view
refreshes and restores the terminal on Ctrl-C. All four app service container IDs
remained unchanged; no application image pins, migrations or data were changed.

## Accounts

### Password recovery release — 2026-09-12, 19:10 Pacific

Auth is deployed as `rck:16291d4` from pushed `main` revision `16291d4`.
Only auth was recreated; both core containers and the persistent edge retained
their container IDs, with green still active. The previous auth image
`rck:bb81666` remains available for rollback by restoring `AUTH_IMAGE` and
recreating only auth with `--no-deps`.

Pre-release backup `backups/20260913T020907Z.dump` passed the disposable restore
check (four migrations, five rooms, four characters). Prior image pins are in
`/srv/rck/releases/16291d4/`. Auth readiness and the public login page show
“Reset it by email” linking to `/forgot`. Resend env vars were already present
on the Pi before this rollout.

### Login statistics release — 2026-09-09, 08:51 Pacific

Auth is deployed as `rck:bb81666` from pushed `main` revision `bb81666`.
Only auth was recreated; both core containers and the persistent edge retained
their container IDs, with green still active. The previous auth image
`rck:6d0e7d0` remains available for rollback by restoring `AUTH_IMAGE` and
recreating only auth with `--no-deps`.

Pre-release backup `backups/20260909T155027Z.dump` passed the disposable restore
check (four migrations, five rooms, three characters). Release binaries and the
protected prior environment are in `/srv/rck/releases/bb81666/`. Core restart
tests, the full race-enabled Go suite, and `go vet ./...` passed. All production
services were healthy after rollout; auth readiness and the public account
redirect passed. `rck-admin stats` reported three registered users and three
distinct users with recorded logins, including in the preceding 24 hours and
seven days.

Statistics use the existing account `last_login` timestamp. They include all
accounts (including operators), count successful password authentication, and
do not establish that a user entered gameplay. Signup alone does not necessarily
record a login; these are account login counts, not a dedicated tester cohort.

On the Pi in `/srv/rck`:

```sh
docker compose exec -T auth /app/rck-admin list
docker compose exec -T auth /app/rck-admin stats
docker compose exec -T auth /app/rck-admin disable USERNAME
docker compose exec -T auth /app/rck-admin enable USERNAME
```

To enable in-game contributions, have the contributor create a normal account, then grant builder permission:

```sh
docker compose exec -T auth /app/rck-admin grant-builder USERNAME
```

Builders use `script new`, `script edit`, and `script test` in their terminal. They can edit only their own drafts and cannot change the live world. An admin uses `script list` and `script show <id>` to review, `script publish <id> <revision>` to approve the reviewed source, and `script attach <id> <target-id|here>` to activate it on an object. There is no submission queue yet; contributors share the script ID with the reviewer.

Use `grant-admin USERNAME` only for trusted reviewers. `revoke-builder USERNAME` or `revoke-admin USERNAME` takes effect on the next scripting operation, including a save from an open editor. Removing builder permission does not remove a separate admin permission. `script disable <id>` stops an already-published script; revoking its author's access alone does not unpublish approved content. See [the scripting guide](SCRIPTING.md) for the full workflow and limits.

For a password reset when email recovery is unavailable, or as an operator
fallback, use Bash and a hidden prompt, keeping the password out of command
arguments and history:

```bash
read -r -s -p 'New password: ' rck_password
printf '\n'
printf '%s\n' "$rck_password" | docker compose exec -T auth /app/rck-admin reset-password USERNAME
unset rck_password
```

Passwords must be 8–72 bytes. Resetting a password invalidates browser sessions on their next authenticated request. Disabling an account also rejects its next terminal command; password resets alone do not disconnect an existing terminal session. CLI actions print an outcome; retain operator records where needed. There is no comprehensive durable moderation audit system yet.

### Resend email recovery

Self-serve `/forgot` and `/reset` send mail through Resend when configured.
Without `RESEND_API_KEY`, those routes return 404 and the login page tells
players to contact the operator.

1. Create a Resend account and API key (`re_…`).
2. Add domain `mail.sid64.quest` in Resend. Disable open and click tracking for transactional mail.
3. Publish the DNS records Resend shows (DKIM CNAMEs and related SPF/verification records) at the `sid64.quest` registrar.
4. Wait until the domain is verified; send a dashboard test message.
5. On the Pi, edit mode-600 `/srv/rck/.env` and set:

```bash
RESEND_API_KEY=re_…
MAIL_FROM=SID64 Quest <noreply@mail.sid64.quest>
```

6. Restart auth (`docker compose up -d auth` from `/srv/rck`, or the usual systemd unit). Confirm https://sid64.quest/login links to `/forgot`, request a reset for a test account, and complete `/reset`.

Reset links expire after one hour and are single-use. Keep `rck-admin reset-password` for mail outages.

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

The alpha uses Tailscale Serve, not Funnel. Preserve other Serve routes. To disable only the MUD web route, use `sudo tailscale serve --https=8443 off`; do not reset the whole Serve configuration. Telnet traffic is protected by the tailnet tunnel, while browser credentials use HTTPS. Before public hosting, revisit transport protection, abuse limits, monitoring and moderation. Password email recovery is available when Resend is configured (see above).

## Release checks

`scripts/alpha-smoke.py` exercises real HTTPS signup/pairing and both live terminal ports, shared presence/chat, duplicate login prevention, reconnect and saved location. It creates disposable accounts listed in `build/pi/smoke-users.json`; delete only those exact test accounts after checking results. Automated socket tests cannot establish CCGMS visual quality or real-hardware behavior. Use [the tester guide](ALPHA-TESTERS.md) for those checks.

## Hardware access update

Terminal listeners now use `MUD_BIND_IP=0.0.0.0` on the Pi for LAN and tailnet access. HTTPS account management remains tailnet-only. No router forwarding was added. For the Commodore on the Mac's Internet Sharing subnet, use the SSH relay described in [VICE testing](VICE-TESTING.md): `192.168.2.1:26464`. This reaches the deployed Pi, not the older local development server.

## Public endpoint activation — 2026-09-08

`sid64.quest` resolves to EC2 at `52.34.32.84`. Caddy is enabled with public HTTPS; HAProxy forwards ports 2323 and 6464 to the Pi. AUTH_BASE_URL is now `https://sid64.quest`. Public users and QR-scanning phones no longer need Tailscale. Earlier private-only instructions describe the previous deployment. The existing Pi ts.net routes are preserved.


## Scripting release — 2026-09-08

Starlark scripting and migration `003_scripting.sql` are deployed. The final release used pre-deploy backup `20260908T221603Z.dump` and image `sha256:c0ae7182a2c97dbb39f6ff4d4f83c79c559211d3907e5c2a55c2ca510336a7de`. Public HTTPS signup/pairing and the contributor workflow were verified through both ANSI and PETSCII: draft editing, isolated preview, builder restrictions, admin publication, attachment, live execution, immediate revocation, disable and detach. Verification used disposable accounts and an isolated room; all test data was removed.

The first attempt stopped at a checksum mismatch in the original migration. The previous image was restored, and the exact `001_initial.sql` was recovered from its embedded copy: its original project-name comment and trailing blank line must remain unchanged. No recorded database checksum was rewritten. The deploy script now checks applied migration hashes before replacing services. Live verification also exposed an unset legacy account field; scripting now uses the authenticated character's owner, with regression coverage.
