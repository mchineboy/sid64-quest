#!/usr/bin/env bash
# Forward the Mac Internet Sharing interface to the Pi PETSCII listener.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$root/build/hardware"
control="$root/build/hardware/ssh.sock"
case "${1:-start}" in
 start)
  if ssh -S "$control" -O check tyler.hardison@symptom-pi 2>/dev/null; then exit 0; fi
  ssh -fNT -M -S "$control" -o BatchMode=yes -o ExitOnForwardFailure=yes \
    -o ServerAliveInterval=30 -o ServerAliveCountMax=3 \
    -L 192.168.2.1:26464:127.0.0.1:6464 tyler.hardison@symptom-pi
  echo 'PETSCII relay: 192.168.2.1:26464 -> symptom-pi:6464'
  ;;
 stop) ssh -S "$control" -O exit tyler.hardison@symptom-pi ;;
 status) ssh -S "$control" -O check tyler.hardison@symptom-pi ;;
 *) echo 'Usage: hardware-relay.sh [start|stop|status]' >&2; exit 1 ;;
esac
