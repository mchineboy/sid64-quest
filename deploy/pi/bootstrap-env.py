#!/usr/bin/env python3
"""Create fresh Pi-only credentials; never overwrite an existing deployment."""
import os
import secrets
from pathlib import Path
path = Path('/srv/rck/.env')
settings = {
 'POSTGRES_HOST': 'postgres', 'POSTGRES_PORT': '5432',
 'POSTGRES_DB': 'race_condition_kingdom', 'POSTGRES_USER': 'mud_user',
 'POSTGRES_PASSWORD': secrets.token_hex(32), 'POSTGRES_SSL_MODE': 'disable',
 'POSTGRES_MAX_CONNS': '10', 'REDIS_HOST': 'redis', 'REDIS_PORT': '6379',
 'REDIS_PASSWORD': secrets.token_hex(32), 'REDIS_POOL_SIZE': '5',
 'MONGODB_URI': '', 'HOST': '0.0.0.0', 'HTTP_PORT': '8080',
 'TELNET_PORT': '2323', 'PETSCII_PORT': '6464', 'MAX_CONNECTIONS': '32',
 'IDLE_TIMEOUT': '900', 'AUTH_BASE_URL': 'https://sid64.quest',
 'AUTH_SECRET_KEY': secrets.token_hex(32), 'BCRYPT_COST': '12',
 'MUD_BIND_IP': '100.67.213.90',
}
try:
 fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
 print('Existing .env preserved.')
else:
 with os.fdopen(fd, 'w') as f:
  for key, value in settings.items(): f.write(f'{key}={value}\n')
  f.flush(); os.fsync(f.fileno())
 print('Created private deployment credentials.')
