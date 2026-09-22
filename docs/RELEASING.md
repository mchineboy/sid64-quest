# Production releases

Publishing a **stable GitHub Release** for a `vMAJOR.MINOR.PATCH` tag deploys
that tagged commit to modeburner. Drafts, prereleases, branch pushes, and bare
tag pushes do not deploy. Use a new stable release rather than promoting an
already-published prerelease. Version tags must point to commits on `main`.

1. Merge/push changes to `main` and wait for **Checks** to pass.
2. In GitHub, create a release with a new tag such as `v0.1.1`, targeting the
   intended commit on `main`. Review the notes, leave **pre-release** unchecked,
   and publish. The CLI equivalent is `gh release create v0.1.1 --target COMMIT
   --title v0.1.1 --notes-file release-notes.md`.
3. Watch **Production release** in Actions and the **production** environment.
   A published release alone does not mean deployment succeeded. Success means
   the deployment job and its public health checks passed.

The workflow tests the exact tagged commit on a fresh GitHub-hosted ARM64 VM
with disposable PostgreSQL/Redis. Tests include the race detector, credential
revocation, database migrations, actual edge/core process replacement, vet,
and a production dependency vulnerability scan. It builds six Linux ARM64
binaries with the Go version in `go.mod` and attaches a reproducible tarball
and SHA-256 checksum to the release. Do not edit, move, or reuse a published tag.

No Actions runner is installed on modeburner or the public proxy. A fixed-function
systemd pull agent on modeburner checks GitHub every two minutes over outbound
HTTPS. Once the tested release artifacts are attached, it verifies GitHub's asset
digest, the tagged commit's ancestry on `main`, and the package manifest before
calling the deployment receiver. It needs no GitHub token, SSH key, or new inbound
port. Repository writers can authorize production application replacement by
publishing a release; restrict write access accordingly.

GitHub waits for `https://sid64.quest/deployment-status.json` to report completion
for the exact version and commit. This read-only endpoint exposes only version,
commit, status and timestamp. It uses the existing HTTPS/Tailscale route to a
loopback status service; production secrets and logs are never returned.

## Deployment behavior

The root-owned `/usr/local/lib/rck-release/receive.py` runs as the existing
deployment account, called locally by the pull agent. It accepts only `verify`
or `deploy` via its fixed command environment and an archive on stdin.
It rejects unexpected paths, symlinks, duplicate entries, oversized payloads,
wrong-architecture binaries, checksum mismatches, changed applied migrations,
version downgrades, and reused versions with different contents. A host lock and
workflow concurrency prevent overlapping deployments. GitHub concurrency can
replace an older *pending* run with a newer one; it does not cancel a running
deployment, and the receiver rejects out-of-order downgrades.

Before cutover it checks current health, builds the image, and takes a database
backup with a disposable restore test. The inactive core must become healthy
before the target changes. Auth and the other core slot are then updated, and
the operator binary link advances. The edge is replaced only when its binary
checksum changes; **an edge replacement disconnects terminal sockets**. Core-only
switches preserve sockets. PostgreSQL and Redis are not replaced by releases.
Public HTTPS, local terminal listeners, and all container health checks must
pass, followed by another backup/restore check.

The receiver does not execute scripts or install configuration files from an
uploaded archive. Compose, Caddy, Dockerfile, and receiver fingerprints must match
the operator-installed versions. Infrastructure changes therefore fail closed
until installed deliberately on the appropriate hosts. The Caddy fingerprint on
the Pi records the approved proxy configuration; it is not a live proxy drift
monitor. Existing proxy routing and host security settings remain separate from
application release automation.

Release records are in `/srv/rck/releases/vMAJOR.MINOR.PATCH/`: the manifest,
binaries, `env.before` (private), target before cutover, and `deployment.json`
with status, backup names, edge restart status, and image ID. The last successful
manifest is `/srv/rck/release-current.json`. Backups remain private on the Pi;
existing off-device backup procedures still apply. Never attach database dumps
or saved environments to public GitHub releases.

## Failures and recovery

A failed candidate does not switch the active core. Later failures may leave a
partially updated stack; inspect the preserved record and fix forward. Automatic
schema rollback is deliberately excluded. A failed version directory blocks
silent retries; an operator must inspect and archive that failed attempt before
retrying, or publish a higher corrective version. A completed identical version
can be rerun as a health-checked no-op. An older version cannot be redeployed
automatically; publish a new version containing a reviewed compatible revert.

Inspect without printing secrets:

```sh
cd /srv/rck
docker compose ps
cat release-current.json
cat releases/v0.1.0/deployment.json
docker compose exec -T edge cat /control/target
```

## Installed services

- `/usr/local/lib/rck-release/` contains root-owned `pull.py`, `receive.py`,
  `status.py`, approved `Dockerfile`/`Caddyfile`, and `min-version` (`v0.1.1`).
  The bootstrap floor excludes `v0.1.0`, whose SSH deployment was blocked by AWS
  ingress rules before any production change. Tags remain immutable.
- `rck-release-pull.timer` checks every two minutes; `rck-release-pull.service`
  runs under the existing deployment user with a 35-minute timeout. Its journal
  contains deployment logs. No downloaded repository script is executed.
- `rck-release-status.service` binds only `127.0.0.1:8091`. Tailscale Serve adds
  `/deployment-status.json` on the existing port 8443. Existing auth and tracker
  routes remain intact. This service does not accept deployment requests.
- `/srv/rck/release-status.json` is the public-safe result of the latest attempted
  release. A failed/interrupted attempt is not retried automatically; inspect its
  logs and state, then publish a newer corrective version or repair deliberately.
- GitHub's `production` environment permits only `v*` tags and requires no second
  approval after publishing. Tag rules prevent updates/deletions of `v*` tags.
  It contains no production deployment credentials.

Inspect or pause automation:

```sh
sudo systemctl status rck-release-pull.timer rck-release-status.service
sudo journalctl -u rck-release-pull.service --since today
sudo systemctl stop rck-release-pull.timer
```

Stopping the timer does not interrupt a deployment already running. Restore it
with `sudo systemctl start rck-release-pull.timer`. Agent, receiver, route and
service changes are deliberate operator installations from this repository.
After an interrupted deployment, verify the actual stack before clearing or
archiving its attempt records. Do not assume an old status timestamp means the
release succeeded.
