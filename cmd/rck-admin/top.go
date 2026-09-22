package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
)

// Select only operator-visible fields: checkpoints also contain authentication
// tokens, pairing codes and pending player output, which must not be displayed.
const topQuery = `SELECT id::text,
 COALESCE(checkpoint->>'Username',''),
 COALESCE(checkpoint->'Character'->>'name',''),
 COALESCE(checkpoint->'Room'->>'name',''),
 COALESCE(checkpoint->>'State',''),
 COALESCE(checkpoint->>'Presentation',''),
 GREATEST(0, EXTRACT(EPOCH FROM (now()-last_seen)))::float8
 FROM terminal_sessions
 WHERE NOT closed AND last_seen > now()-interval '2 minutes'
 ORDER BY lower(COALESCE(checkpoint->>'Username','')), id`

type topSession struct {
	id, user, character, room, state, mode string
	heartbeat                              float64
}

func readTop(ctx context.Context, db *sql.DB) ([]topSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, topQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []topSession{}
	for rows.Next() {
		var s topSession
		if err := rows.Scan(&s.id, &s.user, &s.character, &s.room, &s.state, &s.mode, &s.heartbeat); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func topState(s string) string {
	switch s {
	case "0", "1":
		return "username"
	case "2":
		return "pairing"
	case "3":
		return "character"
	case "4":
		return "playing"
	case "5":
		return "disconnected"
	default:
		return "unknown"
	}
}

// Untrusted account/world text must not inject terminal controls or table rows.
func topText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	runes := []rune(s)
	if len(runes) > limit {
		s = string(runes[:limit-1]) + "~"
	}
	if s == "" {
		return "-"
	}
	return s
}

func renderTop(w io.Writer, sessions []topSession, now time.Time, width, height int) {
	playing, ansi, petscii := 0, 0, 0
	for _, s := range sessions {
		if s.state == "4" {
			playing++
		}
		if s.mode == "0" {
			ansi++
		}
		if s.mode == "1" {
			petscii++
		}
	}
	fmt.Fprintf(w, "SID64 Quest | %s | %d sessions, %d playing\n", now.Format("15:04:05"), len(sessions), playing)
	fmt.Fprintf(w, "ANSI: %d  PETSCII: %d | Ctrl-C to exit\n", ansi, petscii)
	fmt.Fprintln(w, "BEAT = heartbeat age, not player idle time; stale after 2m.")
	fmt.Fprintln(w)
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	wide := width >= 110
	if wide {
		fmt.Fprint(table, "SESSION\t")
	}
	fmt.Fprintln(table, "USER\tCHARACTER\tSTATE\tMODE\tBEAT\tROOM")
	limit := len(sessions)
	if height > 0 && limit > height-7 {
		limit = max(0, height-7)
	}
	for _, s := range sessions[:limit] {
		mode := "?"
		if s.mode == "0" {
			mode = "ANSI"
		}
		if s.mode == "1" {
			mode = "PETSCII"
		}
		if wide {
			fmt.Fprintf(table, "%s\t", topText(s.id, 9))
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%.0fs\t%s\n", topText(s.user, 12), topText(s.character, 12), topState(s.state), mode, s.heartbeat, topText(s.room, 22))
	}
	table.Flush()
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No active terminal sessions.")
	}
	if limit < len(sessions) {
		fmt.Fprintf(w, "... %d more; enlarge terminal or use --once.\n", len(sessions)-limit)
	}
}

func runTop(db *sql.DB, args []string) error {
	flags := flag.NewFlagSet("top", flag.ContinueOnError)
	once := flags.Bool("once", false, "print one snapshot, suitable for scripts")
	interval := flags.Duration("interval", 2*time.Second, "refresh interval (minimum 1s)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected top arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *interval < time.Second {
		return fmt.Errorf("top interval must be at least 1s")
	}
	size, terminalErr := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	live := !*once && terminalErr == nil
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if live {
		fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l")
		defer fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[?1049l")
	}
	for {
		sessions, err := readTop(ctx, db)
		if ctx.Err() != nil {
			return nil
		}
		if live {
			fmt.Fprint(os.Stdout, "\x1b[H\x1b[2J")
		}
		if err != nil {
			if !live {
				return fmt.Errorf("read terminal sessions: %w", err)
			}
			fmt.Fprintln(os.Stdout, "SID64 Quest | session data unavailable; retrying | Ctrl-C to exit")
			fmt.Fprintln(os.Stdout, topText(err.Error(), 180))
		} else {
			width, height := 120, 0
			if live {
				if resized, e := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); e == nil {
					size = resized
				}
				width, height = int(size.Col), int(size.Row)
			}
			renderTop(os.Stdout, sessions, time.Now(), width, height)
		}
		if !live {
			return nil
		}
		timer := time.NewTimer(*interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
