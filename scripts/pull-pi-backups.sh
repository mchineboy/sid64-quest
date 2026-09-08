#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
umask 077
mkdir -p "$root/backups/pi"
# Archives are immutable; copy only complete dumps and checksums.
rsync -a --ignore-existing -e 'ssh -o BatchMode=yes -o ConnectTimeout=15' \
 --include='*.dump' --include='*.dump.sha256' --exclude='*' \
 tyler.hardison@symptom-pi:/srv/rck/backups/ "$root/backups/pi/"
cd "$root/backups/pi"
for checksum in *.sha256; do
 [ -f "$checksum" ] || continue
 # Pi's checksums have a backups/ prefix; verify against the local filenames.
 sed 's@  backups/@  @' "$checksum" | shasum -a 256 -c -
done
# Keep two months of off-device archives.
find . -type f -name '*.dump*' -mtime +60 -delete
