#!/bin/sh
# Restore into a disposable database; never replace the live world.
set -eu
cd /srv/rck
archive=${1:?usage: restore-check.sh backups/TIMESTAMP.dump}
case "$archive" in backups/*.dump) ;; *) echo 'Choose a backup inside /srv/rck/backups' >&2; exit 1;; esac
exec 9>backups/.restore-check.lock
flock -n 9 || exit 1
scratch="rck_restore_check_$(date +%s)_$$"
cleanup() { docker compose exec -T postgres dropdb -U mud_user --if-exists "$scratch"; }
trap cleanup EXIT HUP INT TERM
docker compose exec -T postgres createdb -U mud_user "$scratch"
docker compose exec -T postgres pg_restore -U mud_user -d "$scratch" --exit-on-error < "$archive"
docker compose exec -T postgres psql -U mud_user -d "$scratch" -v ON_ERROR_STOP=1 -c 'SELECT COUNT(*) AS migrations FROM schema_migrations; SELECT COUNT(*) AS rooms FROM rooms; SELECT COUNT(*) AS characters FROM characters;'
echo 'Restore check passed; live database unchanged.'
