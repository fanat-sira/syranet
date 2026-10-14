package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/node"
	"github.com/fanat-sira/syranet/internal/service"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const testTelegramID = 1000001

func environment(t *testing.T) (*config.Config, *database.Store) {
	t.Helper()
	if os.Getenv("SYRANET_E2E") != "disposable" {
		t.Skip("run scripts/e2e.py; never loads production .env")
	}
	c, e := config.Load("/test/config.yaml")
	if e != nil {
		t.Fatal(e)
	}
	s, e := database.New(context.Background(), c.Database, zap.NewNop())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	if e = s.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	return c, s
}
func TestAccessPipeline(t *testing.T) {
	c, s := environment(t)
	ctx := context.Background()
	p, e := service.Credential()
	if e != nil {
		t.Fatal(e)
	}
	sp, _ := service.Credential()
	u, e := s.UpsertTelegramUser(ctx, database.TelegramProfile{TelegramID: testTelegramID}, "e2e_user", p, "e2e_socks", sp)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateQuestionnaire(ctx, testTelegramID, "fixture", "fixture", true); e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveUser(ctx, 1, u.ID, 1); e != nil {
		t.Fatal(e)
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	r, e := service.NewReconciler(c, s)
	if e != nil {
		t.Fatal(e)
	}
	echo := &http.Server{Addr: ":18080", Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host, _, _ := net.SplitHostPort(req.RemoteAddr)
		io.WriteString(w, host)
	}), ReadHeaderTimeout: time.Second}
	listener, e := net.Listen("tcp", echo.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go echo.Serve(listener)
	defer echo.Close()
	ips, e := net.LookupHost("tester")
	if e != nil || len(ips) == 0 {
		t.Fatal("cannot resolve test target")
	}
	target := "http://" + net.JoinHostPort(ips[0], "18080")
	c.Nodes[0].Outbound = "direct"
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	entryIPs, _ := net.LookupHost("node")
	source, e := connect(t, c.Nodes[0], u.HY2Password, target)
	if e != nil || !contains(entryIPs, source) {
		t.Fatal("single-node TCP did not exit from entry", e)
	}
	c.Nodes[0].Outbound = "upstream"
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	upIPs, _ := net.LookupHost("upstream")
	source, e = connect(t, c.Nodes[0], u.HY2Password, target)
	if e != nil || !contains(upIPs, source) {
		t.Fatal("multi-node TCP did not exit from upstream", e)
	}
	// Cause a real child startup failure by colliding SOCKS with the node API.
	// sing-box check succeeds, restart fails, and the previous data plane must recover.
	c.Nodes[0].Socks = config.Socks{Enabled: true, ListenAddress: "0.0.0.0", Port: 8080, AllowPublic: true}
	if r.Sync(ctx) == nil {
		t.Fatal("real bind failure not reported")
	}
	if source, e = connect(t, c.Nodes[0], u.HY2Password, target); e != nil || !contains(upIPs, source) {
		t.Fatal("previous config did not recover after failed restart", e)
	}
	c.Nodes[0].Socks.Enabled = false
	if e = r.Sync(ctx); e != nil {
		t.Fatal("rollback recovery did not reconcile", e)
	}
	// Desired state must survive a rejected apply and reconcile without another admin action.
	if e = s.SuspendUser(ctx, 1, u.ID); e != nil {
		t.Fatal(e)
	}
	oldToken := r.Clients["entry"].Token
	r.Clients["entry"].Token = "deliberately_invalid_test_token"
	if r.Sync(ctx) == nil {
		t.Fatal("sync failure hidden")
	}
	states, _ := s.SyncStates(ctx)
	failed := false
	for _, v := range states {
		if v.NodeID == "entry" && !v.Healthy && v.Error != "" {
			failed = true
		}
	}
	if !failed {
		t.Fatal("failed desired state not persisted")
	}
	r.Clients["entry"].Token = oldToken
	if e = r.Sync(ctx); e != nil {
		t.Fatal("reconciliation did not recover", e)
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e == nil {
		t.Fatal("suspended password connected")
	}
	if e = s.ActivateUser(ctx, 1, u.ID); e != nil {
		t.Fatal(e)
	}
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e != nil {
		t.Fatal("activation failed", e)
	}
	if e = s.Action(ctx, 1, u.ID, "revoke", 0, ""); e != nil {
		t.Fatal(e)
	}
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e == nil {
		t.Fatal("revoked password connected")
	}
	if e = s.Action(ctx, 1, u.ID, "rotate", 0, ""); e != nil {
		t.Fatal(e)
	}
	if e = s.ActivateUser(ctx, 1, u.ID); e != nil {
		t.Fatal(e)
	}
	u, _ = s.GetUserByID(ctx, u.ID)
	if u.HY2Password == p {
		t.Fatal("credential not rotated")
	}
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = connect(t, c.Nodes[0], p, target); e == nil {
		t.Fatal("old password survives rotation")
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e != nil {
		t.Fatal("rotated password fails", e)
	}
	// Force expiry through an explicitly isolated database, never a configured production DB.
	conn, e := pgx.Connect(ctx, c.DSN())
	if e != nil {
		t.Fatal(e)
	}
	_, e = conn.Exec(ctx, "UPDATE users SET expires_at=NOW()-INTERVAL '1 second' WHERE id=$1", u.ID)
	conn.Close(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExpireUsers(ctx); e != nil {
		t.Fatal(e)
	}
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e == nil {
		t.Fatal("expired password connected")
	}
	if e = s.ExtendUser(ctx, 1, u.ID, 1, ""); e != nil {
		t.Fatal(e)
	}
	if e = r.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = connect(t, c.Nodes[0], u.HY2Password, target); e != nil {
		t.Fatal("extension failed", e)
	}
	// Simulate process loss with the same persistent state using node status/API.
	status, e := r.Clients["entry"].Status(ctx)
	if e != nil || !status.Running || !status.TLSReady {
		t.Fatal("entry not ready at completion")
	}
}
func TestDatabasePersistence(t *testing.T) {
	c, s := environment(t)
	u, e := s.GetUserByTelegramID(context.Background(), testTelegramID)
	if e != nil || !u.IsSubscriptionActive(time.Now()) {
		t.Fatal("database access lost after restart", e)
	}
	r, e := service.NewReconciler(c, s)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Sync(context.Background()); e != nil {
		t.Fatal("node did not recover after restart", e)
	}
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func connect(t *testing.T, n config.Node, password, target string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	listen, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return "", e
	}
	port := listen.Addr().(*net.TCPAddr).Port
	listen.Close()
	spec := map[string]any{"log": map[string]any{"level": "error"}, "inbounds": []any{map[string]any{"type": "socks", "listen": "127.0.0.1", "listen_port": port}}, "outbounds": []any{map[string]any{"type": "hysteria2", "tag": "proxy", "server": n.PublicAddress, "server_port": n.Port, "password": password, "tls": map[string]any{"enabled": true, "server_name": n.TLS.ServerName, "certificate_path": "/certs/ca.pem"}}}, "route": map[string]any{"final": "proxy"}}
	b, _ := json.Marshal(spec)
	path := filepath.Join(dir, "client.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		return "", e
	}
	client := &node.Process{Binary: "/usr/local/bin/sing-box"}
	if e = client.Check(context.Background(), path); e != nil {
		return "", e
	}
	if e = client.Start(path); e != nil {
		return "", e
	}
	defer client.Stop()
	proxy, _ := url.Parse("socks5://127.0.0.1:" + strconv.Itoa(port))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	httpClient := &http.Client{Transport: tr, Timeout: 4 * time.Second}
	res, e := httpClient.Get(target)
	if e != nil {
		return "", fmt.Errorf("test tunnel connection failed")
	}
	defer res.Body.Close()
	body, e := io.ReadAll(res.Body)
	return string(body), e
}
