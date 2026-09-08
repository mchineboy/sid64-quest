# Testing with VICE and CCGMS

The launcher runs an NTSC C64 with user-port RS232 at 2400 baud, connected over
IP232 to a local tcpser Hayes modem. Dialing `555` connects to the MUD's
PETSCII listener at `127.0.0.1:6464`.

## Setup and launch

VICE (`x64sc`), a C compiler, Git, make, and lsof are required. On this Mac,
Homebrew VICE 3.10 is installed. The setup script builds a pinned upstream
tcpser revision inside the ignored `build/vice/` directory.

```sh
scripts/vice-setup.sh
scripts/vice-test.sh /absolute/path/to/CCGMS.d64
```

Without a disk argument, the launcher opens C64 BASIC so a disk can be
attached later. Quit that test VICE instance before launching another.
Closing VICE also stops the modem process started by the launcher.

The launcher attaches a working copy as drive 8. Each launch with a disk
argument replaces that working copy; copy it elsewhere first if you want to
preserve CCGMS settings or phonebook edits. The supplied disk is untouched.
VICE starts from defaults and does not save settings on exit.

Start the local MUD dependencies and services as described in the README.
For the Pi instead, use:

```sh
MUD_ADDRESS=symptom-pi:6464 scripts/vice-test.sh /absolute/path/to/CCGMS.d64
```

`VICE_BIN` and `TCPSER_BIN` can override executable paths. The modem's IP232
port (25232) and unused incoming-call port (26400) bind only to loopback.

## Inside CCGMS

1. At BASIC, enter `LOAD"*",8,1`, then `RUN`. If CCGMS is not the first file,
   use its filename instead of `*`.
2. Select the user-port/Hayes modem driver and 2400 baud. Exact menu names
   vary by CCGMS version; do not select SwiftLink or UP9600 for this profile.
3. Choose 40-column PETSCII/Commodore graphics mode. Turn local echo off:
   tcpser echoes modem commands and the MUD echoes connected input.
4. In terminal mode, enter `AT`. Expect `OK`.
5. Dial `ATDT555` (or `ATDT127.0.0.1:6464`). Expect `CONNECT`, then the MUD.
6. Enter a username. Open the displayed pairing URL in the Mac browser,
   sign in or register, then wait for CCGMS to advance and select a character.

## Playtest checklist

- Welcome and pairing screens: native graphics/color, readable code, no
  missing text or unexpected extra blank rows.
- Input: Return submits once; delete edits correctly; no double echo.
- Commands: `look`, `where`, `help`, `who`, `stats`, `inventory`.
- Travel: `e`, then `w`; exits and location should agree.
- Objective: speak to the Town Crier, retrieve the manifest, return it.
- Reconnect: `quit`, dial again, authenticate, verify saved progress.
- Display recovery: `terminal petscii` redraws the current screen.

Logs are `build/vice/vice.log` and `build/vice/tcpser.log`. `NO CARRIER` on
dial usually means the MUD isn't listening at the selected address. An
unresponsive `AT` suggests the CCGMS modem driver or baud rate doesn't match.
Avoid warp mode during serial testing.

## Verification so far

On 2026-09-08, tcpser built successfully on this Mac, VICE 3.10 initialized
its C64, and tcpser confirmed VICE's IP232 connection and DTR assertion.
Both helper listeners were verified as loopback-only. CCGMS disk loading,
modem commands, and end-to-end MUD play remain to be verified with a disk image.

References: [VICE RS232 setup](https://vice-emu.pokefinder.org/wiki/RS232),
[tcpser upstream](https://github.com/go4retro/tcpser), and
[CCGMS Future](https://github.com/mist64/ccgmsterm).

CCGMS 2021 note: use NTSC timing for this profile. PAL user-port timing fixes
were added in CCGMS Future 0.1 (2022). Matching baud settings alone did not
prevent framing errors in the initial PAL test. NTSC retesting is pending.

## Mixed-case PETSCII

The server selects the lowercase/uppercase character set with CHR$(14) (`0x0e`) on each full screen. Text retains its original case, including names, descriptions, URLs and pairing codes. Lowercase letters transmit as `0x41–0x5a`; uppercase as `0xc1–0xda`. Input also accepts the uppercase aliases `0x61–0x7a`. Keep CCGMS in native PETSCII mode. Verify `say Hello from CCGMS!` reaches an ANSI player with exactly that case.

## Real Commodore on Mac Internet Sharing

Current hardware address: `192.168.2.2`; Mac sharing gateway: `192.168.2.1`.
Run `scripts/hardware-relay.sh start` on the Mac, then dial **192.168.2.1:26464** from the Commodore modem. This byte-transparent SSH tunnel reaches the Pi PETSCII port over Tailscale. Port 26464 avoids the older local development gateway on 6464. The Commodore needs no Tailscale client; complete browser pairing on the Mac using the displayed HTTPS URL.

The Mac must remain awake, with Internet Sharing and Tailscale connected. Use `scripts/hardware-relay.sh status` or `stop`; rerun `start` after a Mac reboot or failed tunnel. The relay is not installed as a login service.

The Pi terminal ports also now bind all IPv4 interfaces (`MUD_BIND_IP=0.0.0.0`), allowing direct LAN access at its current address `192.168.86.42:6464`. No router port forwarding or public endpoint was configured.

## Scan-to-login prototype

At the pairing screen, type `QR`. The server renders the session's short `/p/CODE` URL using C64 quadrant blocks, packing four QR modules into each 8x8 character. Both character sets contain the needed shapes. The screen retains a four-module white quiet zone and rejects URLs that require more than 21 character rows. Use a black terminal background; the server selects white foreground. `HELP` restores text instructions, and `CHECK` completes sign-in.

The quadrant output was rendered with VICE's actual C64 character ROM and independently decoded using macOS Vision to the exact test URL. This establishes digital rendering correctness; a phone scan of the real CCGMS display still needs testing. The phone must have access to the private Tailscale HTTPS endpoint. No external URL shortener is used.

## Public endpoint activation — 2026-09-08

`sid64.quest` resolves to EC2 at `52.34.32.84`. Caddy is enabled with public HTTPS; HAProxy forwards ports 2323 and 6464 to the Pi. AUTH_BASE_URL is now `https://sid64.quest`. Public users and QR-scanning phones no longer need Tailscale. Earlier private-only instructions describe the previous deployment. The existing Pi ts.net routes are preserved.
