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

No Actions runner is installed on modeburner or the public proxy. The final job
uses the `production` environment's `RELEASE_SSH_KEY` secret and
`RELEASE_KNOWN_HOSTS` variable to reach a forced command on `loop.seraphnet.com`.
The proxy forwards the archive over Tailscale to a second forced command on the
Pi. Both keys prohibit shells, PTYs, agent forwarding, and port forwarding.
The CI key still authorizes production application replacement; restrict
repository write access accordingly. Personal SSH keys and production `.env`
values are never uploaded to GitHub.

## Deployment behavior

The root-owned `/usr/local/lib/rck-release/receive.py` runs as the existing
deployment account. It accepts only `verify` or `deploy` and an archive on stdin.
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

## Installed SSH path

- Proxy: root-owned `/usr/local/bin/rck-release-forward`; restricted CI public
  key in `/home/admin/.ssh/authorized_keys`. A separate private key and pinned Pi
  host key live in `rck-production` and `rck-production-known-hosts` in that directory.
- Pi: restricted proxy public key in the deployment account's `authorized_keys`;
  receiver and approved `Dockerfile`/`Caddyfile` in `/usr/local/lib/rck-release/`.
- GitHub: the `production` environment permits only `v*` tags, has the dedicated
  CI secret, and requires no second approval after publishing a release. Tag
  rules prevent updates/deletions of `v*` tags.

To rotate credentials, provision a fresh pair at each hop, replace the matching
restricted public key and GitHub environment secret, verify host pins, and remove
the old keys. Preserve the forced-command restrictions. Receiver/forwarder
updates are operator installations from this repository, followed by `verify`
with a newly packaged release before deployment.
