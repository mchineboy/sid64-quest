#!/bin/sh
# Run the independently released operator binary using the auth service's
# existing network/environment. Does not recreate or restart any service.
set -eu
cd /srv/rck
if [ -t 0 ] && [ -t 1 ]; then
    exec docker compose run --rm --no-deps --entrypoint /ops/rck-admin \
        -v /srv/rck/admin/current/rck-admin:/ops/rck-admin:ro auth "$@"
fi
exec docker compose run --rm --no-deps -T --entrypoint /ops/rck-admin \
    -v /srv/rck/admin/current/rck-admin:/ops/rck-admin:ro auth "$@"
