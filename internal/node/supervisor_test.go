package node

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeEngine struct {
	running                  bool
	checks, starts           int
	failCheck, failNextStart bool
}

func (f *fakeEngine) Check(context.Context, string) error {
	f.checks++
	if f.failCheck {
		return errors.New("secret output")
	}
	return nil
}
func (f *fakeEngine) Start(string) error {
	f.starts++
	if f.failNextStart {
		f.failNextStart = false
		return errors.New("secret")
	}
	f.running = true
	return nil
}
func (f *fakeEngine) Stop() error   { f.running = false; return nil }
func (f *fakeEngine) Running() bool { return f.running }
func spec() Spec {
	var s Spec
	s.Log.Level = "error"
	s.Inbounds = []Inbound{{Type: "hysteria2", Tag: "hy2", Listen: "::", Port: 443, Users: []User{{Name: "user_1", Password: strings.Repeat("a", 64)}}, TLS: &TLS{Enabled: true, ServerName: "vpn.example.com", CertificatePath: "/certs/fullchain.pem", KeyPath: "/certs/privkey.pem"}}}
	s.Outbounds = []Outbound{{Type: "direct", Tag: "egress"}}
	s.Route.Final = "egress"
	return s
}
func supervisor(t *testing.T) (*Supervisor, *fakeEngine) {
	f := &fakeEngine{}
	s := New(t.TempDir(), f)
	s.Certificate = func(Spec) (string, error) { return "certificate", nil }
	return s, f
}
func TestApplyRollbackRetryAndRestart(t *testing.T) {
	s, f := supervisor(t)
	ctx := context.Background()
	a := spec()
	if e := s.Apply(ctx, a); e != nil {
		t.Fatal(e)
	}
	first := s.Status().Hash
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	a.Inbounds[0].Port = 444
	f.failCheck = true
	if s.Apply(ctx, a) == nil || f.starts != 1 {
		t.Fatal("invalid candidate applied")
	}
	f.failCheck = false
	f.failNextStart = true
	if s.Apply(ctx, a) == nil {
		t.Fatal("restart failure hidden")
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if string(before) != string(after) || s.Status().Hash != first || !f.running {
		t.Fatal("rollback failed")
	}
	if e := s.Apply(ctx, a); e != nil || s.Status().Hash == first {
		t.Fatal("retry failed")
	}
	f.running = false
	if e := s.Apply(ctx, a); e != nil || !f.running {
		t.Fatal("dead process not restarted")
	}
	info, _ := os.Stat(filepath.Join(s.Dir, "config.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions unsafe")
	}
}
func TestConcurrentApplySerializes(t *testing.T) {
	s, f := supervisor(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.Apply(context.Background(), spec()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if f.starts != 1 {
		t.Fatal("identical applies restarted process")
	}
}
func TestAuthenticatedAPI(t *testing.T) {
	s, _ := supervisor(t)
	token := strings.Repeat("t", 64)
	server := httptest.NewServer(Handler(s, token))
	defer server.Close()
	r, e := http.Get(server.URL + "/status")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("unauthorized access")
	}
	c, _ := NewClient(server.URL, token, "")
	if _, e = c.Apply(context.Background(), spec()); e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("PUT", server.URL+"/config", strings.NewReader(`{"log":{"level":"debug"},"secret":"must-not-be-returned"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("unknown fields accepted")
	}
}
func TestTLSFailureFailsClosed(t *testing.T) {
	s, f := supervisor(t)
	s.Certificate = func(Spec) (string, error) { return "", errors.New("private key contents") }
	e := s.Apply(context.Background(), spec())
	if e == nil || strings.Contains(e.Error(), "contents") || f.starts != 0 {
		t.Fatal("unsafe TLS fallback")
	}
}

func TestCanonicalSOCKSHTTPAndCertificateRenewal(t *testing.T) {
	s, f := supervisor(t)
	token := strings.Repeat("t", 64)
	server := httptest.NewServer(Handler(s, token))
	defer server.Close()
	c, _ := NewClient(server.URL, token, "")
	v := spec()
	v.Inbounds = append(v.Inbounds, Inbound{Type: "socks", Tag: "socks", Listen: "127.0.0.1", Port: 1080, Users: []SocksUser{{Username: "u", Password: strings.Repeat("b", 64)}}})
	encoded, _ := Encode(v)
	status, e := c.Apply(context.Background(), v)
	if e != nil || status.Hash != Hash(encoded) {
		t.Fatal("HTTP changed canonical config hash")
	}
	s.Certificate = func(Spec) (string, error) { return "renewed-certificate", nil }
	if s.Status().TLSReady {
		t.Fatal("certificate change not detected")
	}
	if e = s.Apply(context.Background(), v); e != nil || f.starts != 2 || !s.Status().TLSReady {
		t.Fatal("renewed certificate was not applied")
	}
}

func TestRemoteTLSVerification(t *testing.T) {
	s, _ := supervisor(t)
	token := strings.Repeat("t", 64)
	server := httptest.NewTLSServer(Handler(s, token))
	defer server.Close()
	c, _ := NewClient(server.URL, token, "")
	if _, e := c.Status(context.Background()); e == nil {
		t.Fatal("untrusted remote certificate accepted")
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	cert := server.TLS.Certificates[0].Certificate[0]
	os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600)
	c, e := NewClient(server.URL, token, path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Status(context.Background()); e != nil {
		t.Fatal("trusted remote certificate rejected", e)
	}
}
