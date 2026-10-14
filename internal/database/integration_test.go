package database

import (
	"context"
	"errors"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Only connect to an explicitly configured disposable test database, never .env.
func TestPostgresLifecycle(t *testing.T) {
	if os.Getenv("SYRANET_TEST_DATABASE") != "disposable" {
		t.Skip("requires disposable database and SYRANET_TEST_DATABASE=disposable")
	}
	ctx := context.Background()
	dbPort := 5432
	if v := os.Getenv("TEST_DB_PORT"); v != "" {
		dbPort, _ = strconv.Atoi(v)
	}
	cfg := config.DatabaseConfig{Host: os.Getenv("TEST_DB_HOST"), Port: dbPort, Name: "syranet_test", User: "syranet_test", Password: os.Getenv("TEST_DB_PASSWORD"), SSLMode: "disable", MaxOpenConns: 10, MinOpenConns: 1, MaxConnLifetime: time.Minute}
	if cfg.Host == "" || cfg.Password == "" {
		t.Fatal("missing TEST_DB_HOST/TEST_DB_PASSWORD")
	}
	adminCfg := config.Config{Database: cfg}
	adminCfg.Database.Name = "postgres"
	conn, e := pgx.Connect(ctx, adminCfg.DSN())
	if e != nil {
		t.Fatal(e)
	}
	_, e = conn.Exec(ctx, "CREATE DATABASE syranet_test")
	conn.Close(ctx)
	if e != nil {
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "42P04" {
			t.Fatal(e)
		}
	}
	s, e := New(ctx, cfg, zap.NewNop())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal("migration not idempotent:", e)
	}
	id := time.Now().UnixNano()%1_000_000_000 + 1
	u, e := s.UpsertTelegramUser(ctx, TelegramProfile{TelegramID: id}, "u_test", strings.Repeat("a", 64), "s_test", strings.Repeat("b", 64))
	if e != nil {
		t.Fatal(e)
	}
	defer s.pool.Exec(ctx, "DELETE FROM users WHERE id=$1", u.ID)
	if e = s.UpdateQuestionnaire(ctx, id, "test", "test", true); e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveUser(ctx, 1, u.ID, 30); e != nil {
		t.Fatal(e)
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	expiry := *u.ExpiresAt
	if e = s.ApproveUser(ctx, 1, u.ID, 30); e != nil {
		t.Fatal(e)
	}
	again, _ := s.UpsertTelegramUser(ctx, TelegramProfile{TelegramID: id}, "new-name", "new-secret", "new-socks", "new-secret")
	if !again.ExpiresAt.Equal(expiry) || again.HY2Password != u.HY2Password {
		t.Fatal("repeated start/approval changed access")
	}
	request := WithRequest(ctx, "integration-extend-"+time.Now().Format(time.RFC3339Nano))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.ExtendUser(request, 1, u.ID, 5, "") }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	if !u.ExpiresAt.Equal(expiry.AddDate(0, 0, 5)) {
		t.Fatal("replayed command extended twice")
	}
	if e = s.SuspendUser(ctx, 1, u.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateQuestionnaire(ctx, id, "reset", "reset", true); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("blocked questionnaire accepted")
	}
	// Simulate audit failure: whole user update must roll back.
	if _, e = s.pool.Exec(ctx, `CREATE OR REPLACE FUNCTION test_reject_audit() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'test audit failure'; END; $$ LANGUAGE plpgsql; CREATE TRIGGER test_reject_audit BEFORE INSERT ON admin_logs FOR EACH ROW EXECUTE FUNCTION test_reject_audit()`); e != nil {
		t.Fatal(e)
	}
	e = s.ActivateUser(ctx, 1, u.ID)
	s.pool.Exec(ctx, "DROP TRIGGER test_reject_audit ON admin_logs; DROP FUNCTION test_reject_audit()")
	if e == nil {
		t.Fatal("audit failure hidden")
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	if u.Status != "suspended" {
		t.Fatal("update not rolled back with audit")
	}
	if e = s.ActivateUser(ctx, 1, u.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Action(ctx, 1, u.ID, "revoke", 0, ""); e != nil {
		t.Fatal(e)
	}
	users, e := s.ListConfigUsers(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range users {
		if v.ID == u.ID {
			t.Fatal("revoked access still configured")
		}
	}
	old := u.HY2Password
	if e = s.Action(ctx, 1, u.ID, "rotate", 0, ""); e != nil {
		t.Fatal(e)
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	if old == u.HY2Password || len(u.HY2Password) != 64 {
		t.Fatal("rotation failed")
	}
	s.pool.Exec(ctx, "UPDATE users SET status='active',expires_at=NOW()-INTERVAL '1 second' WHERE id=$1", u.ID)
	if _, e = s.ExpireUsers(ctx); e != nil {
		t.Fatal(e)
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	if u.Status != "expired" {
		t.Fatal("expiry failed")
	}
	if e = s.ActivateUser(ctx, 1, u.ID); e == nil {
		t.Fatal("expired activated")
	}
}
