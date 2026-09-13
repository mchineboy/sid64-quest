# SID64 Quest alpha tester guide

Public testing is available at https://sid64.quest/signup. No Tailscale installation is needed.

1. Create an account at https://sid64.quest/signup and create a character. The account page manages email, password and up to five characters, including names. Gameplay happens in the terminal.
2. Connect to `sid64.quest` port **2323** for ANSI, or **6464** for PETSCII. Use the full domain name, sid64.quest.
3. Enter your username. Open the displayed pairing URL in your browser and sign in. The terminal advances automatically; choose your character when prompted. Type `renew` if the pairing code expires.
4. Try `help`, `look`, `who`, `say hello`, and `east`. Explore, collect and use items, and talk to NPCs. Type `quit` when finished.

A real Commodore modem can dial `sid64.quest:6464` directly over the internet. During pairing, type `QR` to scan the login URL with a phone, or `HELP` for text instructions. The Mac relay remains optional. See [VICE testing](VICE-TESTING.md) for client setup.

Please test login and character creation, the 40-column login/character/room screens, ANSI-to-PETSCII chat, reconnecting after a disconnect, and whether position and inventory survive logout. A character can have only one active connection. Idle sessions close after 15 minutes; an abruptly lost network connection may take time to expire.

Report the client and version, ANSI or PETSCII mode, time and timezone, exact commands, expected result, actual result, and a screenshot when useful. Never include passwords or pairing links/codes. Report issues directly to the operator; no invitations or issue reports are sent automatically.

This is a small starter world. When Resend is configured on the server, use **Forgot your password?** on the sign-in page to reset by email. Otherwise recovery is operator-assisted. There is no account deletion or web gameplay. The Pi starts with a fresh world and does not import local development accounts.

Hardware-test update: the Pi now also accepts direct LAN terminal connections. The configured Commodore at `192.168.2.2` can reach the Pi through the Mac relay at **192.168.2.1:26464**. Keep the Mac awake and perform HTTPS pairing there. See the VICE testing guide for relay start/stop instructions.

Use `take all` (or `get all`) to gather available item stacks. Your pack holds 50 items; quest items are limited to one of each. Anything you cannot carry stays in the room. Try taking scenery, too: the fountain and lamppost have opinions about being portable.
