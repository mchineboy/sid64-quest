#!/bin/sh
# Root-owned forced command on the proxy. The CI key cannot open a shell,
# allocate a PTY, forward ports/agents, or supply its own remote command.
set -eu
case "${SSH_ORIGINAL_COMMAND:-}" in
  verify|deploy) mode=$SSH_ORIGINAL_COMMAND ;;
  *) echo 'Only verify and deploy are supported' >&2; exit 1 ;;
esac
exec /usr/bin/ssh -T -i /home/admin/.ssh/rck-production \
  -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/home/admin/.ssh/rck-production-known-hosts \
  -o ConnectTimeout=20 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 \
  tyler.hardison@symptom-pi "$mode"
