#!/usr/bin/env bash
# Run after tests pass. Upload only explicit release files, never the local .env.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
# Applied migrations are immutable, including comments and whitespace. Check
# before touching the running image so drift cannot take the service offline.
migration_checksums=$(ssh tyler.hardison@symptom-pi 'cd /srv/rck && docker compose exec -T postgres psql -X -U mud_user -d race_condition_kingdom -At -F " " -c "SELECT version,checksum FROM schema_migrations ORDER BY version"')
while read -r version expected; do
  [[ -z "$version" ]] && continue
  if [[ ! "$version" =~ ^[0-9]+_[a-zA-Z0-9_]+\.sql$ ]]; then
    echo "Invalid applied migration name: $version" >&2
    exit 1
  fi
  migration="internal/database/migrations/$version"
  if [[ ! -f "$migration" ]]; then
    echo "Missing applied migration: $migration" >&2
    exit 1
  fi
  actual=$(shasum -a 256 "$migration" | awk '{print $1}')
  if [[ "$actual" != "$expected" ]]; then
    echo "Applied migration checksum differs: $version. Restore the deployed file before releasing." >&2
    exit 1
  fi
done <<< "$migration_checksums"
export CGO_ENABLED=0 GOOS=linux GOARCH=arm64
mkdir -p build/pi
for app in auth-service telnet-gateway rck-admin; do go build -o "build/pi/$app" "./cmd/$app"; done
ssh tyler.hardison@symptom-pi 'cd /srv/rck && ./backup.sh'
scp build/pi/auth-service build/pi/telnet-gateway build/pi/rck-admin deploy/pi/Dockerfile tyler.hardison@symptom-pi:/srv/rck/image/
scp deploy/pi/compose.yml deploy/pi/backup.sh deploy/pi/restore-check.sh tyler.hardison@symptom-pi:/srv/rck/
ssh tyler.hardison@symptom-pi 'cd /srv/rck && docker tag rck:alpha rck:previous && docker build -t rck:alpha image && docker compose up -d --wait --wait-timeout 180'
