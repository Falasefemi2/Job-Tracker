package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/Falasefemi2/jobtracker/internal/db"
)

func main() {
	_ = godotenv.Load()
	if err := run(context.Background(), os.Args[1:], os.Getenv("DATABASE_URL"), "migrations"); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, databaseURL, dir string) error {
	mode := "up"
	if len(args) > 0 {
		mode = args[0]
	}
	if mode != "up" && mode != "down" {
		return errors.New("usage: migrate [up|down]")
	}
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	// Reuse the shared opener so migrations get the same connection handling
	// (pool sizing, pooler query-mode workaround) as the rest of the app.
	database, err := db.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no migration files in %s", dir)
	}

	applied := map[string]bool{}
	rows, err := database.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	switch mode {
	case "up":
		for _, f := range files {
			version := strings.TrimSuffix(filepath.Base(f), ".up.sql")
			if applied[version] {
				continue
			}
			if err := applyFile(ctx, database, f, version, true); err != nil {
				return err
			}
			log.Printf("migrated up: %s", version)
		}
	case "down":
		// Roll back only the latest applied migration.
		last := ""
		for _, f := range files {
			version := strings.TrimSuffix(filepath.Base(f), ".up.sql")
			if applied[version] && version > last {
				last = version
			}
		}
		if last == "" {
			log.Print("nothing to roll back")
			return nil
		}
		down := filepath.Join(dir, last+".down.sql")
		if _, err := os.Stat(down); err != nil {
			return fmt.Errorf("missing down file %s: %w", down, err)
		}
		if err := applyFile(ctx, database, down, last, false); err != nil {
			return err
		}
		log.Printf("migrated down: %s", last)
	}
	return nil
}

func applyFile(ctx context.Context, db *sql.DB, path, version string, up bool) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("apply %s: %w", path, err)
	}
	if up {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record %s: %w", version, err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
			return fmt.Errorf("unrecord %s: %w", version, err)
		}
	}
	return tx.Commit()
}
