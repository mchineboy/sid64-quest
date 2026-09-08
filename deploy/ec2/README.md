# Public gateway preparation

Host: admin@loop.seraphnet.com, SSH key ~/.ssh/seraphnet.pem.
Inspected Debian 13 ARM64, approximately 1 GiB RAM and 25 GiB free disk.
Existing SSH and loopback port 2222 tunnel must be preserved.
On 2026-09-08, both configurations were validated with the installed Debian binaries and installed. HAProxy is enabled and running; forwarding on 2323/6464 returns Pi terminal data when tested from EC2. Caddy is stopped and disabled pending public DNS and security-group readiness. Original package configurations are preserved as *.pre-mud. EC2 Tailscale IP: 100.94.32.35. Current public IPv4: 52.34.32.84; Elastic IP status is not yet confirmed.

Activation sequence:
1. Install Tailscale and authorize this host in the same tailnet as symptom-pi.
2. Restrict gateway tailnet access to the Pi's TCP ports 8443, 2323 and 6464.
3. Confirm a stable EC2 public IPv4 (prefer an Elastic IP) and point sid64.quest's A record at it. Do not use the Pi's 100.x address for public DNS.
4. Permit inbound TCP 80, 443, 2323 and 6464 in the instance security group. Preserve SSH access and avoid exposing databases. Review IPv6 separately before publishing AAAA records.
5. Install Caddy and HAProxy; validate Caddyfile and haproxy.cfg with their installed binaries before activation. Copy these files to the distribution's configuration locations only after checking for existing configurations.
6. Test HTTPS through the gateway, preserving the Pi's original Tailscale Serve routes. The HTTPS upstream uses the Pi's existing valid ts.net certificate; do not disable certificate verification.
7. Change only AUTH_BASE_URL on the Pi to https://sid64.quest; restart its auth and gateway services. Test generated pairing links and QR scans from outside Tailscale.
8. Exercise both terminal modes from an external network, then check reboot recovery of gateway services.

Public terminals are plaintext on the client-to-EC2 segment, including game chat. Passwords stay in the HTTPS browser flow. The EC2-to-Pi segment uses Tailscale. The current application sees shared proxy source addresses: review HTTP rate limits for concurrent testers before increasing the group size. No PROXY protocol is sent to the existing terminal server.

Public traffic and availability still depend on the Pi and its home connection. Resolve its observed SD-card stalls before relying on the service. Monitor EC2 transfer usage and public IPv4 charges in AWS.

Public-port verification: after the EC2 security-group update, direct connections from the Mac to 52.34.32.84:2323 and :6464 received terminal data. Ports 80/443 refused connections while Caddy remained stopped. sid64.quest did not resolve; public HTTPS and public pairing are pending DNS. Existing generated login URLs still require Tailscale.

## Public endpoint activation — 2026-09-08

`sid64.quest` resolves to EC2 at `52.34.32.84`. Caddy is enabled with public HTTPS; HAProxy forwards ports 2323 and 6464 to the Pi. AUTH_BASE_URL is now `https://sid64.quest`. Public users and QR-scanning phones no longer need Tailscale. Earlier private-only instructions describe the previous deployment. The existing Pi ts.net routes are preserved.

## Terminal transport keep-alive

HAProxy enables TCP keep-alive independently on the client and Pi connections:
30 seconds idle before probing, 10 seconds between unanswered probes, and nine
unanswered probes before declaring a dead peer. Successful probes are invisible
to ANSI and raw PETSCII clients. The edge's own TCP keep-alive only covers its
connection to HAProxy, not the public client connection. See the
[HAProxy 3.0 keep-alive documentation](https://docs.haproxy.org/3.0/configuration.html#4.2-option%20tcpka).

These transport probes do not override the edge's intentional 15-minute player
idle logout or HAProxy's 16-minute application inactivity timeout. Do not claim
they can repair a lost network or a modem's independent idle/disconnect policy.

Validate with `sudo haproxy -c -f /etc/haproxy/haproxy.cfg`, then use
`sudo systemctl reload haproxy` (not restart). Existing connections drain in the
old worker without being disconnected; only new connections get new socket
settings. Verify both public ports and inspect `sudo ss -tnopi` for
`timer:(keepalive,...)` on both proxy legs. Keep a backup of the prior config.

Applied on 2026-09-08 with HAProxy 3.0.11 validation and a graceful reload.
The existing PETSCII connection remained established in the old worker. New
ANSI/PETSCII connections showed keep-alive timers on both proxy legs and
successful client-side probes with unchanged application byte counts. Both
public sockets survived 75 seconds without client input and answered input
afterwards. No edge/core restart or database change was needed. This verifies
the server behavior, not the cause of a particular modem's “no carrier” report.
Previous config: `/etc/haproxy/haproxy.cfg.pre-keepalive-20260908`.

Repeat the idle smoke check from outside the tailnet:

```sh
python3 scripts/test-terminal-keepalive.py --host sid64.quest --seconds 75
```
