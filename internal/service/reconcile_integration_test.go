package service

import (
	"context"
	"fmt"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/node"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type testEngine struct {
	running bool
	starts  int
}

func (f *testEngine) Check(context.Context, string) error { return nil }
func (f *testEngine) Start(string) error                  { f.running = true; f.starts++; return nil }
func (f *testEngine) Stop() error                         { f.running = false; return nil }
func (f *testEngine) Running() bool                       { return f.running }
func TestPostgresReconciliation(t *testing.T) {
	if os.Getenv("SYRANET_TEST_DATABASE") != "disposable" {
		t.Skip("explicit disposable PostgreSQL required")
	}
	port := 5432
	if p := os.Getenv("TEST_DB_PORT"); p != "" {
		port, _ = strconv.Atoi(p)
	}
	db := config.DatabaseConfig{Host: os.Getenv("TEST_DB_HOST"), Port: port, Name: "syranet_test", User: "syranet_test", Password: os.Getenv("TEST_DB_PASSWORD"), SSLMode: "disable", MaxOpenConns: 10, MinOpenConns: 1, MaxConnLifetime: time.Minute}
	ctx := context.Background()
	store, e := database.New(ctx, db, zap.NewNop())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	f := &testEngine{}
	sup := node.New(t.TempDir(), f)
	sup.Certificate = func(node.Spec) (string, error) { return "test-cert", nil }
	server := httptest.NewServer(node.Handler(sup, strings.Repeat("a", 64)))
	defer server.Close()
	c, _ := fixture()
	c.Database = db
	c.Nodes[0].Agent = config.Agent{URL: server.URL, Token: strings.Repeat("a", 64)}
	rec, e := NewReconciler(c, store)
	if e != nil {
		t.Fatal(e)
	}
	id := time.Now().UnixNano()%1_000_000_000 + 1
	p, _ := Credential()
	user, e := store.UpsertTelegramUser(ctx, database.TelegramProfile{TelegramID: id}, fmt.Sprint("r_", id), p, fmt.Sprint("s_", id), p)
	if e != nil {
		t.Fatal(e)
	}
	store.UpdateQuestionnaire(ctx, id, "test", "test", true)
	if e = store.ApproveUser(ctx, 1, user.ID, 30); e != nil {
		t.Fatal(e)
	}
	defer func() {
		conn, _ := pgx.Connect(ctx, c.DSN())
		if conn != nil {
			conn.Exec(ctx, "DELETE FROM users WHERE id=$1", user.ID)
			conn.Close(ctx)
		}
	}()
	if e = rec.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	first := sup.Status().Hash
	// Independent managers share a DB advisory lock; identical snapshots must not restart.
	other, _ := NewReconciler(c, store)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				errs <- rec.Sync(ctx)
			} else {
				errs <- other.Sync(ctx)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if f.starts != 1 {
		t.Fatal("concurrent unchanged snapshots restarted node")
	}
	if e = store.SuspendUser(ctx, 1, user.ID); e != nil {
		t.Fatal(e)
	}
	rec.Clients["entry"].Token = "invalid"
	if rec.Sync(ctx) == nil {
		t.Fatal("rejected apply hidden")
	}
	states, _ := store.SyncStates(ctx)
	pending := false
	for _, s := range states {
		if s.NodeID == "entry" {
			pending = !s.Healthy && s.Error != "" && s.AppliedHash == first
		}
	}
	if !pending {
		t.Fatal("desired/applied mismatch missing")
	}
	rec.Clients["entry"].Token = strings.Repeat("a", 64)
	if e = rec.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if sup.Status().Hash == first {
		t.Fatal("suspended user not reconciled")
	}
	// A disabled node is actively drained instead of being silently skipped.
	c.Nodes[0].Enabled = false
	if e = rec.Sync(ctx); e != nil {
		t.Fatal(e)
	}
}
