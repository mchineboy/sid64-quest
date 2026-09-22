# Security remediation

This change addresses SEC-01 through SEC-06 in the 2026-09-22 review. It was deployed on 2026-09-22 as `rck:security-20260922-d9721f1`; see the [release record](ALPHA-RUNBOOK.md#security-release--2026-09-22).

## Changes

- The public proxy asserts the client IP with a separate shared credential. The app authenticates the assertion in constant time and rejects missing/invalid assertions when proxy mode is enabled. It never trusts ordinary forwarded headers. Direct local HTTP continues to use the TCP peer. Browser, pairing, and API logins share per-client and per-account limits; account password checks use the authenticated account identity.
- Anonymous forms use HMAC-authenticated cookies with a 15-minute expiry and bound CSRF nonce. Cookie-free GET requests allocate no Redis state. Authenticated browser sessions remain server-side and rotate on login. The Pi Redis template also uses a memory ceiling and `noeviction` to protect critical session keys.
- Character-name validation is shared at the service layer. Stored character labels are stripped of terminal control characters before game presentation, including legacy names/checkpoints. The obsolete development password hint is removed, and pairing pages have an anti-framing policy.
- Migration 008 adds a credential version and a trigger that increments it whenever the password, email, or active flag changes. This also covers direct operator SQL updates. New terminal sessions store the version captured during authentication. Commands, legacy idle checks, and persistent-core polls/replays revalidate it. Old checkpoints without a version require reauthentication.
- Recovery grants carry the credential version. Successful recovery updates the password only if that version still matches, with the database trigger advancing it in the same operation. Sibling, stale, and concurrent recovery links cannot overwrite newer credentials. Legacy reset links require a new recovery request. Recovery lookup fails closed for ambiguous historical email identities.
- The Go minimum and vulnerable crypto/text/compression/MongoDB dependencies are updated. Release binaries must be rebuilt with a patched toolchain; changing only the runtime image does not update Go libraries.
- PostgreSQL connection strings use URL encoding so empty passwords and reserved characters cannot alter database selection or other connection fields.

## Deployment preparation

1. Use Go 1.26.8 or newer patched tooling for the release. The validation runs used `GOTOOLCHAIN=go1.26.8`. Existing local `GOTOOLCHAIN=local` settings require installing a sufficiently new toolchain or explicitly choosing one.
2. Generate a distinct random `AUTH_PROXY_TOKEN` of at least 32 characters (for example, 32 random bytes encoded as hex). Store the same value in the Pi's private `/srv/rck/.env` and in Caddy's protected service environment on EC2. Do not reuse `AUTH_SECRET_KEY`, commit either value, or place the proxy credential in client URLs. Existing bootstrap environments are intentionally preserved; they must receive the new variable explicitly.
3. Keep `AUTH_SECRET_KEY` stable and at least 32 characters on the auth service. It signs anonymous forms; an ephemeral key is used only for direct local development without a configured strong key. Rotating it expires anonymous forms.
4. Install the updated Caddyfile, validate it with the credential in the service environment, and reload Caddy first. Caddy overwrites both `X-RCK-Client-IP` and `X-RCK-Proxy-Token`; the HTTPS/Tailscale forwarding path must preserve these two headers. The old auth binary ignores the additional headers, so this order is compatible. Do not enable trust in arbitrary incoming `X-Forwarded-For` or `X-RCK-*` values at Caddy.
5. Back up the database and deploy the updated auth and game binaries using the existing release procedure. Migration 008 is additive and older applied migrations remain unchanged. Existing authenticated terminals reconnect once because their checkpoints lack a version. Do not leave an old core/gateway running after migration: it does not enforce the new version. Rollback to an old binary would reintroduce the vulnerabilities.
6. The Compose auth service now requires `AUTH_PROXY_TOKEN` and `AUTH_SECRET_KEY`; it fails configuration if they are absent. The auth process rejects short or reused proxy secrets. Public account/pairing requests without a valid proxy assertion get 403; only GET health/readiness probes bypass the assertion.
7. Verify through the full public proxy path: two distinct clients have independent quotas, spoofed incoming headers are overwritten, and browser login/pairing/recovery work. Confirm password changes disconnect existing ANSI and PETSCII terminals, including idle sessions and sessions surviving a core replacement. Direct tailnet account access that bypasses the authenticating Caddy proxy is no longer supported in proxy mode; health probes remain available internally.

The shared proxy credential is safe only on the existing verified-TLS path to the Pi and loopback/container path to auth. Keep auth's host port bound to loopback. This change does not encrypt the intentionally plaintext legacy telnet client connection or establish a hard per-script memory sandbox; those additional boundaries from the review still apply.

## Verification

Regression tests cover authenticated proxy isolation and spoofing, login limits across API/browser paths, rejection and escaping of terminal controls, zero Redis allocation for anonymous pages, anonymous-cookie tampering/expiry, stale/sibling/concurrent reset links, credential changes by direct SQL, and revocation after core replacement.

The integration suite was run with disposable PostgreSQL and Redis services on loopback, including the actual core/edge process-restart test. No production data or running deployment was used.

Completed checks:

- Full `go test -p 1 -count=1 ./...` with database/core integration enabled and all restart-test binaries available: passed.
- Security regression tests with `-race`, including concurrent recovery and revoked-command replay: passed.
- `go vet ./...`: passed.
- All command binaries built with `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`: passed.
- `govulncheck` v1.8.0 with the production Linux ARM64 configuration and Go 1.26.8: zero symbol-level or imported-package vulnerabilities. One module-only advisory remains for the unused, unmaintained `golang.org/x/crypto/openpgp` package (GO-2026-5932); the project does not import it and no patched version exists.
- Caddy configuration adaptation and Compose configuration validation with dummy secrets: passed.
- Live public HTTPS signup and ANSI/PETSCII pairing/gameplay: passed. Password change disconnected an idle ANSI session; recovery disconnected an idle PETSCII session and invalidated a sibling recovery grant. Old credentials were rejected and updated credentials worked. Recovery grants were created by the operator for disposable accounts; no email was sent.
- Live Caddy/Tailscale forwarding: two actual clients received separate quota buckets, spoofed client headers were overwritten, and direct requests bypassing the proxy assertion returned 403.
- All deployed services are healthy. Disposable accounts and their scoped session state were removed; four original accounts/characters remain. Pre- and post-release database backups passed disposable restore checks and were copied off-host with verified checksums.

The Pi kernel reports no Docker memory-limit support. Redis's own 256 MiB `maxmemory` and `noeviction` policy are active, but the separate 512 MiB container memory limit cannot be enforced on this host. No host reboot or kernel configuration change was included in this rollout.
