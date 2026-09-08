#!/usr/bin/env python3
"""Exercise a running alpha with two disposable accounts. Prints account names for operator cleanup."""
import http.cookiejar
import json
import os
import re
import secrets
import socket
import time
import urllib.parse
import urllib.request
from pathlib import Path

BASE = os.environ.get('ALPHA_BASE_URL', 'https://symptom-pi.tail8762f9.ts.net:8443')
HOST = urllib.parse.urlparse(BASE).hostname
record=Path("build/pi/smoke-users.json")
previous=json.loads(record.read_text()) if record.exists() else []
users = []
sockets = []
pet_sockets = set()

def pet_encode(text):
    return bytes(ord(c)-32 if "a" <= c <= "z" else ord(c)+128 if "A" <= c <= "Z" else ord(c) for c in text)

def pet_decode(data):
    return bytes(c+32 if 65 <= c <= 90 else c-128 if 193 <= c <= 218 else c for c in data)

def request(browser, path, data=None):
    payload = urllib.parse.urlencode(data).encode() if data is not None else None
    with browser.open(BASE + path, payload, timeout=15) as response:
        return response.geturl(), response.read().decode()

def signup():
    suffix = secrets.token_hex(4)
    username = 'smoke_' + suffix
    name = 'Scout ' + ''.join(chr(ord('a') + int(x, 16)) for x in suffix)
    password = secrets.token_urlsafe(24)
    browser = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    _, body = request(browser, '/signup')
    csrf = re.search(r'name="csrf" value="([^"]+)"', body)[1]
    request(browser, '/signup', dict(csrf=csrf, username=username, email=username+'@example.test', character_name=name, password=password, confirm_password=password))
    users.append(username)
    Path('build/pi/smoke-users.json').write_text(json.dumps(previous+users))
    _, body = request(browser, '/account')
    assert name in body and 'Town Square' in body, 'Signup/account view failed'
    return browser, username, password, name

def until(s, marker, timeout=15):
    end=time.monotonic()+timeout
    result=b''
    while time.monotonic()<end:
        try:
            chunk=s.recv(8192)
            if s in pet_sockets: chunk=pet_decode(chunk)
        except socket.timeout:
            continue
        if not chunk:
            raise RuntimeError('Unexpected disconnect')
        result+=chunk
        if marker(result): return result
    raise RuntimeError('Timed out waiting for terminal response')

def connect(player, port):
    browser,username,password,name=player
    s=socket.create_connection((HOST,port),timeout=10);s.settimeout(.5);sockets.append(s)
    if port == 6464: pet_sockets.add(s)
    until(s,lambda b: b'username' in b.lower())
    s.sendall((pet_encode(username) if port == 6464 else username.encode())+b'\r\n')
    data=until(s,lambda b: re.search(rb'[A-Z0-9]{4}-[A-Z0-9]{4}',b))
    code=re.search(rb'[A-Z0-9]{4}-[A-Z0-9]{4}',data)[0].decode()
    endpoint,_=request(browser,'/p/'+code)
    token=urllib.parse.parse_qs(urllib.parse.urlparse(endpoint).query)['token'][0]
    _,body=request(browser,'/auth',dict(token=token,username=username,password=password))
    assert 'AUTHENTICATION SUCCESSFUL' in body
    s.sendall(pet_encode('check\r') if port == 6464 else b'check\r\n')
    until(s,lambda b: b'character' in b.lower())
    s.sendall(b'1\r\n')
    return s

try:
    a,b=signup(),signup()
    one=connect(a,2323);until(one,lambda d:b'Town Square' in d)
    two=connect(b,6464);until(two,lambda d:b'Town Square' in d)
    one.sendall(b'who\r\n');until(one,lambda d:b[3].encode() in d)
    one.sendall(b'say hello from the alpha test\r\n')
    until(two,lambda d:b'hello from the alpha test' in re.sub(rb'\s+',b' ',d))
    two.sendall(pet_encode('say Hello from CCGMS!\r'))
    until(one,lambda d:b'Hello from CCGMS!' in d)
    duplicate=connect(a,6464);until(duplicate,lambda d:b'already online' in d)
    one.sendall(b'east\r\n');until(one,lambda d:b'Market Lane' in d)
    one.close();time.sleep(.4)
    duplicate.sendall(b'1\r\n');until(duplicate,lambda d:b'Market Lane' in d)
    for s in [two,duplicate]: s.sendall(pet_encode('quit\r'))
    print('PASS: signup, HTTPS pairing, ANSI/PETSCII shared presence/chat, duplicate prevention, disconnect/reconnect, saved location, quit.')
    print('Disposable accounts: '+', '.join(users))
finally:
    for s in sockets:s.close()
