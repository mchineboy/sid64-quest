#!/usr/bin/env bash
# Isolated C64 user-port test rig. Quit VICE to stop its modem bridge.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
state="$root/build/vice"
vice=${VICE_BIN:-x64sc}
tcpser=${TCPSER_BIN:-$state/tcpser-src/tcpser}
if [[ ${1:-} == --help ]]; then
  echo 'Usage: scripts/vice-test.sh [CCGMS.d64]'
  echo 'VICE_BIN and TCPSER_BIN override executables. MUD_ADDRESS defaults to 127.0.0.1:6464.'
  exit 0
fi
if (( $# > 1 )); then echo 'Expected at most one disk image.' >&2; exit 1; fi
command -v "$vice" >/dev/null || { echo 'Install VICE or set VICE_BIN.' >&2; exit 1; }
[[ -x "$tcpser" ]] || { echo 'Run scripts/vice-setup.sh first.' >&2; exit 1; }
mkdir -p "$state"
disk_args=()
if (( $# )); then
  [[ -f "$1" ]] || { echo "Disk image not found: $1" >&2; exit 1; }
  # CCGMS can save settings without altering the supplied original disk.
  cp "$1" "$state/ccgms-test.d64"
  disk_args=(-8 "$state/ccgms-test.d64")
fi
# Refuse a second rig rather than accidentally attach to another modem.
for port in 25232 26400; do
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Port $port is occupied. Close the existing test rig first." >&2
    exit 1
  fi
done
"$tcpser" -v 127.0.0.1:25232 -p 127.0.0.1:26400 -s 2400 -S 2400 \
  -i 's5=20' -n "555=${MUD_ADDRESS:-127.0.0.1:6464}" -l 4 >"$state/tcpser.log" 2>&1 &
modem_pid=$!
cleanup() { kill "$modem_pid" 2>/dev/null || true; wait "$modem_pid" 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sleep 0.5
kill -0 "$modem_pid" 2>/dev/null || { cat "$state/tcpser.log" >&2; exit 1; }
echo 'CCGMS: User Port / Hayes, 2400 baud, 40-column PETSCII, local echo off.'
echo 'Load the attached disk with LOAD"CCGMS 2021",8,1 then RUN. Dial ATDT555.'
echo "Logs: $state"
"$vice" -default -model ntsc +saveres +sound +warp -logfile "$state/vice.log" \
  -rsdev2 127.0.0.1:25232 -rsdev2ip232 -rsuserdev 1 \
  -rsuserbaud 2400 -userportdevice 2 ${disk_args[@]+"${disk_args[@]}"}
