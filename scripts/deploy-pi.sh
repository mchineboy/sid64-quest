#!/usr/bin/env bash
# Run after tests pass. Upload only explicit release files, never the local .env.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
export CGO_ENABLED=0 GOOS=linux GOARCH=arm64
mkdir -p build/pi
for app in auth-service telnet-gateway rck-admin; do go build -o "build/pi/$app" "./cmd/$app"; done
ssh tyler.hardison@symptom-pi 'cd /srv/rck && ./backup.sh'
scp build/pi/auth-service build/pi/telnet-gateway build/pi/rck-admin deploy/pi/Dockerfile tyler.hardison@symptom-pi:/srv/rck/image/
scp deploy/pi/compose.yml deploy/pi/backup.sh deploy/pi/restore-check.sh tyler.hardison@symptom-pi:/srv/rck/
ssh tyler.hardison@symptom-pi 'cd /srv/rck && docker tag rck:alpha rck:previous && docker build -t rck:alpha image && docker compose up -d --wait --wait-timeout 180'
