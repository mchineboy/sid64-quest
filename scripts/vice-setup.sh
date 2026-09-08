#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
source_dir="$root/build/vice/tcpser-src"
revision=fe7feff4862406b277e009d14c219f5d16cf1222
if [[ ! -d "$source_dir" ]]; then
  git clone https://github.com/go4retro/tcpser.git "$source_dir"
  git -C "$source_dir" checkout --detach "$revision"
fi
if [[ $(git -C "$source_dir" rev-parse HEAD) != "$revision" ]]; then
  echo "Expected tcpser revision $revision; inspect $source_dir before rebuilding." >&2
  exit 1
fi
make -C "$source_dir"
echo 'Ready: scripts/vice-test.sh /path/to/CCGMS.d64'
