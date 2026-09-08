#!/bin/sh
set -eu
cd /srv/rck
umask 077
mkdir -p backups
exec 9>backups/.backup.lock
flock -n 9 || exit 0
stamp=$(date -u +%Y%m%dT%H%M%SZ)
partial="backups/$stamp.dump.partial"
trap 'rm -f "$partial"' EXIT HUP INT TERM
docker compose exec -T postgres pg_dump -U mud_user -d race_condition_kingdom -Fc > "$partial"
test -s "$partial"
# Ensure the archive parses before publishing it atomically.
docker compose exec -T postgres pg_restore --list < "$partial" >/dev/null
sync "$partial"
mv "$partial" "backups/$stamp.dump"
sync backups
sha256sum "backups/$stamp.dump" > "backups/$stamp.dump.sha256"
# Keep 30 days locally; off-device copies have separate retention.
find backups -name '*.dump*' -type f -mtime +30 -delete
printf 'Backup complete: %s.dump\n' "$stamp"
