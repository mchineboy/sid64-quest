// ci-init prepares only the disposable loopback database used by Actions tests.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func main() {
	cfg := config.LoadFromEnv()
	if os.Getenv("CI") != "true" || cfg.Database.PostgreSQL.Host != "127.0.0.1" || cfg.Database.PostgreSQL.Password != "ci-only-password" {
		log.Fatal("ci-init requires the disposable CI loopback database")
	}
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := game.NewWorldService(db).EnsureStarterWorld(ctx); err != nil {
		log.Fatal(err)
	}
}
