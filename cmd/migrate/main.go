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

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
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

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
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
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
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
			if err := applyFile(ctx, db, f, version, true); err != nil {
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
		if err := applyFile(ctx, db, down, last, false); err != nil {
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
