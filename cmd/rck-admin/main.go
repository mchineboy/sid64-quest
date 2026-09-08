package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"golang.org/x/crypto/bcrypt"
	"os"
	"strings"
	"time"
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: rck-admin list | disable USER | enable USER | reset-password USER (new password on stdin)")
	}
	cfg := config.LoadFromEnv()
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if os.Args[1] == "list" {
		rows, e := db.QueryContext(ctx, `SELECT username,is_active FROM users ORDER BY username`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var active bool
			if e = rows.Scan(&name, &active); e != nil {
				return e
			}
			fmt.Printf("%s active=%t\n", name, active)
		}
		return rows.Err()
	}
	if len(os.Args) != 3 {
		return fmt.Errorf("provide one username")
	}
	name := strings.ToLower(strings.TrimSpace(os.Args[2]))
	var result sql.Result
	switch os.Args[1] {
	case "disable", "enable":
		result, err = db.ExecContext(ctx, `UPDATE users SET is_active=$1 WHERE username=$2`, os.Args[1] == "enable", name)
	case "reset-password":
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("new password required on stdin")
		}
		password := scanner.Text()
		if len(password) < 8 || len(password) > 72 {
			return fmt.Errorf("password must be 8-72 bytes")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), cfg.Auth.BCryptCost)
		if e != nil {
			return e
		}
		result, err = db.ExecContext(ctx, `UPDATE users SET password_hash=$1 WHERE username=$2`, string(hash), name)
	default:
		return fmt.Errorf("unknown action")
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("account not found")
	}
	fmt.Printf("%s: %s complete\n", name, os.Args[1])
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
