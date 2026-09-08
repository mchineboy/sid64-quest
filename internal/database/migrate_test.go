package database

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"testing"
	"time"
)

func TestMigrationsFreshAndRepeat(t *testing.T) {
	cfg := config.LoadFromEnv()
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("rck_migration_test_%d", time.Now().UnixNano())
	if _, err = db.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DROP DATABASE ` + name)
	cfg.Database.PostgreSQL.Database = name
	fresh, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	for i := 0; i < 2; i++ {
		if err = Migrate(ctx, fresh); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = fresh.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("migrations: %d, %v", count, err)
	}
	if err = fresh.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("fresh database seeded users", err)
	}
	if _, err = fresh.ExecContext(ctx, `UPDATE schema_migrations SET checksum='changed' WHERE version='001_initial.sql'`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, fresh); err == nil {
		t.Fatal("changed checksum accepted")
	}
}
