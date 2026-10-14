package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/fanat-sira/syranet/migrations"
	"github.com/jackc/pgx/v5"
	"sort"
)

func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(731091)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())"); e != nil {
		return e
	}
	entries, e := migrations.Files.ReadDir(".")
	if e != nil {
		return e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, f := range entries {
		b, e := migrations.Files.ReadFile(f.Name())
		if e != nil {
			return e
		}
		h := sha256.Sum256(b)
		checksum := hex.EncodeToString(h[:])
		var existing string
		e = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", f.Name()).Scan(&existing)
		if e == nil {
			if existing != checksum {
				return errors.New("applied migration checksum mismatch")
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return errors.New("SQL migration failed: " + f.Name())
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", f.Name(), checksum); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
