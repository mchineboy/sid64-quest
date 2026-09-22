# Security evaluation — 2026-09-22

Follow-up: fixes for SEC-01 through SEC-06 and their verification are recorded in [Security remediation](SECURITY-REMEDIATION.md). They were deployed on 2026-09-22 as `rck:security-20260922-d9721f1`. The findings below preserve the original assessment before remediation.

Source review of working tree based on commit `d9721f1d5459a94b931ee3a3039e0be12d327983`, including existing uncommitted changes. No application fixes or deployment changes were made. This assessment covers authentication, account recovery, terminal handling, script execution, dependency advisories, and deployment configuration. It is not a live penetration test or an exhaustive audit.

## Findings

| ID | Severity / priority | Finding | Evidence |
| --- | --- | --- | --- |
| SEC-01 | High | One client can exhaust the proxy's shared authentication quota | Isolated reproduction + deployment configuration |
| SEC-02 | Medium | Registration permits stored terminal control sequences in character names | Validation and output independently reproduced |
| SEC-03 | Medium | Password changes and recovery do not revoke terminal access | Source trace; database integration not run |
| SEC-04 | Medium | Outstanding recovery links survive password and email changes | Token behavior reproduced; reset handler reviewed |
| SEC-05 | Medium | Anonymous GET requests allocate unbounded long-lived Redis sessions | Isolated reproduction |
| SEC-06 | Update before next release | Toolchain and dependencies have known advisories | Official govulncheck scan; exploitability varies |

### SEC-01: Shared proxy rate limits permit authentication denial of service

Location: `internal/auth/http.go:17–28`; `internal/account/service.go:160–178`; `deploy/ec2/Caddyfile`; `deploy/pi/compose.yml`.

The middleware uses the TCP peer from `RemoteAddr` as its client identity. In the supplied reverse-proxy deployment, this is the proxy/container gateway, shared by public visitors. After 60 POSTs per minute from one visitor, all visitors behind that peer receive HTTP 429 on every POST route. The middleware increments the quota before token, credential, or CSRF validation, so an attacker does not need an account or valid pairing token. Repeating the requests maintains the outage.

Password recovery has a second shared quota: five requests per 15 minutes per peer. A visitor can obtain a normal anonymous session and exhaust this quota with nonexistent account identities. This blocks other visitors from requesting recovery email for the window.

Validation: an isolated test sent 60 POSTs with one forwarded address, then a POST with a different forwarded address through the same proxy peer. The second visitor received 429. No public service was contacted.

Fix: establish a trusted proxy chain and derive client identity only from headers overwritten by explicitly trusted proxies. Do not blindly trust arbitrary `X-Forwarded-For` values. Enforce separate per-client and per-account limits across all login paths, with a separate global overload budget. Add a test proving one public client cannot consume another's normal quota through the deployed proxy chain.

### SEC-02: Stored terminal control injection through registration

Location: `internal/auth/auth.go:518–541`; `cmd/auth-service/main.go:224–229`; `internal/telnet/connection.go:350–364`; `internal/telnet/server.go:647–650`.

The account UI rejects control characters through `validName`, but the terminal pairing `/register` endpoint calls `RegisterPlayer` directly. That service only trims the character name and checks its length. An attacker with a freely obtained pairing challenge can submit a URL-encoded name such as `Eve%1B%5B2J%1B%5BH`. The name is stored and later written unescaped into ANSI `who`, room/chat messages, and other terminal output.

Impact: persistent screen manipulation and forged terminal presentation for other players. Effects beyond screen manipulation depend on the victim's terminal emulator; remote code execution was not established.

Validation: a closed-database stub showed a name containing ESC sequences passed all service validation and reached `BeginTx`. A separate test verified that `SendWhoList` emitted those bytes unchanged. Persistence through PostgreSQL was traced in source, not exercised.

Fix: move shared name/email/password validation into the service layer so every caller uses it. Reject terminal controls, including DEL/C1 controls, at input boundaries and escape untrusted data at terminal output boundaries without removing intentional server formatting. Review existing stored names before relying on input validation alone.

### SEC-03: Credential recovery leaves existing terminal sessions authorized

Location: `internal/account/http.go:312–323,415–426`; `internal/telnet/server.go:363–370`; `internal/auth/auth.go:632` (`UserActive`).

Browser sessions are checked against a fingerprint of the current password hash. Terminal sessions have no equivalent password version. Changing or resetting a password updates the hash and invalidates browser sessions, while already connected terminals continue to be authorized solely by `users.is_active`.

An attacker who paired a terminal before the victim recovered the account can keep issuing commands afterward. Normal input renews activity, so the idle timeout is not a recovery mechanism. The persistent core also checkpoints terminal state separately from browser sessions.

Fix: record an authentication generation in every terminal session, compare it on commands and polling, and increment it atomically on password reset/change and explicit global logout. Terminate both legacy gateway sessions and checkpointed edge/core sessions when it changes. Checking only Redis session deletion is insufficient where state has already been copied into a connection or checkpoint.

Validation: source trace only. Follow-up integration test: pair a terminal, reset the account from another browser, and assert that both the next command and subsequent poll require reauthentication, including after core restart.

### SEC-04: Old password-reset links remain valid after account recovery

Location: `internal/account/service.go:180–215`; `internal/account/http.go:296–323,391,415`.

Each reset token stores only a user ID for one hour. Consuming one token deletes only that token; other outstanding tokens remain valid. Neither password changes nor email changes revoke them, and the reset update checks only user ID and active status. Thus a previously obtained link can change the password after the legitimate owner changes credentials, uses a different recovery link, or replaces a compromised email address.

Prerequisite: possession of a still-unexpired earlier reset link. This is not a token guessing vulnerability.

Validation: two tokens for one user were issued and independently consumed successfully. Source inspection confirms that successful reset/change does not revoke sibling tokens or compare the password/email state that existed when the token was issued. A complete database-backed recovery reproduction remains pending.

Fix: bind tokens to an account recovery generation or credential version and validate it atomically with the password update. Advance that version on successful recovery, password changes, and recovery-address changes. Ensure two different tokens cannot both succeed concurrently against the same version.

### SEC-05: Unauthenticated GETs cause persistent Redis growth

Location: `internal/account/http.go:109–114`; `internal/account/service.go:33,81–104`; `internal/auth/http.go:17`; `deploy/pi/compose.yml` Redis service.

Every cookie-free `GET /login` or `GET /signup` creates a new Redis-backed anonymous session with a 12-hour lifetime. GET requests have no application rate limit. A client can omit cookies repeatedly and grow the same Redis instance used for pairing and gameplay. The supplied Redis deployment has AOF persistence but no configured memory ceiling, so this can consume memory and disk and impair authentication/game availability.

Validation: 100 requests through the real middleware and account handler created 100 independent keys, each with a 12-hour TTL. No load or exhaustion test was performed; actual exhaustion depends on host capacity and any external limits.

Fix: rate-limit anonymous session issuance, use a shorter anonymous-session TTL or a carefully designed signed anonymous CSRF cookie, and place bounded anonymous state separately from critical gameplay/session storage. Configure capacity and persistence limits without evicting essential live sessions indiscriminately.

### SEC-06: Patch the build toolchain and vulnerable dependencies

The installed toolchain is `go1.26.3 darwin/arm64`. Running `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` with scanner v1.8.0 reported **11 symbol-reachable advisories**, plus three package-level and six module-level advisories without a reported called vulnerable symbol. A call-graph match is not proof that attacker-controlled input reaches an exploitable configuration.

| Advisory | Component | Scanner's first fixed version |
| --- | --- | --- |
| [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) | net/url | Go 1.26.6 |
| [GO-2026-6091](https://pkg.go.dev/vuln/GO-2026-6091) | html/template | Go 1.26.6 |
| [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) | crypto/tls | Go 1.26.6 |
| [GO-2026-6089](https://pkg.go.dev/vuln/GO-2026-6089) | net/http | Go 1.26.6 |
| [GO-2026-5972](https://pkg.go.dev/vuln/GO-2026-5972) | encoding/asn1 | Go 1.26.6 |
| [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) | golang.org/x/text v0.37.0 | v0.39.0 |
| [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856) | crypto/tls | Go 1.26.5 |
| [GO-2026-5327](https://pkg.go.dev/vuln/GO-2026-5327) | mongo-driver v1.17.4 | v1.17.7 |
| [GO-2026-5039](https://pkg.go.dev/vuln/GO-2026-5039) | net/textproto | Go 1.26.4 |
| [GO-2026-5037](https://pkg.go.dev/vuln/GO-2026-5037) | crypto/x509 | Go 1.26.4 |
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | net/http / vendored IDNA | Go 1.26.6 |

Triage: the template issue concerns JavaScript regexp contexts, absent from the reviewed account/pairing templates. The HTTP/2 issue requires unencrypted HTTP/2 support, which is not explicitly enabled in the reviewed servers. MongoDB is optional and initialized only when `MONGODB_URI` is set; the driver advisory concerns GSSAPI. The x/text trace also passes through MongoDB. These conditions prevent treating all 11 reports as proven public exploits.

Fix: use a maintained, patched Go release, at least 1.26.6 on the inspected branch for the listed fixes; update x/text to at least v0.39.0 and mongo-driver to at least v1.17.7, or remove unused MongoDB functionality. Rebuild all release binaries and rescan with the production GOOS/GOARCH/CGO settings. `scripts/deploy-pi.sh:32–34` uses the local `go build`, so changing the runtime Alpine image alone does not patch embedded Go libraries. Production binary versions were not inspected. Add vulnerability scanning to release checks. See the [official Go vulnerability-management documentation](https://go.dev/doc/security/vuln/).

## Additional hardening and accepted boundaries

- Script workers have time, CPU, output, step, and concurrency limits, but `GOMEMLIMIT=128MiB` is a **soft** GC target, not a hard allocation cap. `internal/scripting/limits_linux.go` sets CPU/core limits only, and the supplied Compose files do not configure memory limits. Builder-accessible draft execution can allocate large objects before timeout. Add a hard worker/container memory boundary and verify it on Linux. This was source-reviewed, not stress-tested. The [Go GC guide](https://go.dev/doc/gc-guide) explains the soft limit. The existing memory test checks eventual failure, not peak memory or host survival.
- Public telnet transport is plaintext by design. HTTPS browser login keeps passwords off that socket, but does not protect game traffic or pairing-code integrity against an active network attacker. Document the boundary and provide an encrypted connection option where clients support one.
- Recovery identity lookup lowercases email, but the database uniqueness constraint is case-sensitive. Normalize addresses consistently before storage and enforce matching uniqueness semantics so recovery cannot ambiguously match multiple accounts.
- Pairing pages lack the account handler's anti-framing CSP. Add a compatible `frame-ancestors 'none'` policy, and remove the obsolete `admin / admin123` development hint. Current migrations do not seed that account.
- `.env` is ignored and untracked. The runbook explicitly records historical development credentials in Git history; their current reuse/rotation was not independently verified. No secret values were copied into this report.

## Existing protections observed

Parameterized SQL; ownership checks for account character changes; current database checks for builder/admin actions; atomic Lua consumption of pairing challenges; random browser/reset tokens; password-fingerprinted browser sessions; CSRF validation and restrictive CSP on account pages; bounded HTTP bodies and terminal input; credential-free Starlark subprocess environments with no module loader; bearer authentication for core requests; private core/database networking; non-root, read-only application containers with dropped capabilities.

## Validation and limitations

- `go test -count=1 ./...`: passed after permitting local sockets. The first sandboxed attempt failed because local socket creation was denied.
- `go vet ./...`: passed.
- Five temporary focused tests passed: shared proxy bucket, anonymous Redis growth, sibling reset-token validity, control-character name reaching database access, and control-character name surviving terminal output. Temporary test files were removed after review.
- PostgreSQL-backed account/recovery/migration tests skipped because the default local credentials failed. Core integration tests require `RCK_CORE_INTEGRATION=1` and were not enabled. Linux worker resource behavior was not exercised on macOS.
- The dependency scan used the local build configuration and the official Go vulnerability database on the review date. Container image packages and deployed binaries were not scanned.
- No public endpoint probes, real recovery email, production data writes, or deployments were performed. Existing user edits were preserved.

Recommended order: fix the shared proxy quota and terminal input validation; implement credential/recovery generation checks; bound anonymous session allocation; rebuild with patched dependencies. Validate each change with isolated regression tests and then database-backed integration tests in a disposable environment.
