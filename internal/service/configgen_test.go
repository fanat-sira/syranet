package service

import (
	"encoding/json"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/node"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture() (*config.Config, models.User) {
	n := config.Node{ID: "entry", Name: "My server", Enabled: true, Entry: true, PublicAddress: "2001:db8::1", ListenAddress: "::", Port: 443, TLS: config.TLS{ServerName: "vpn.example.com", CertificatePath: "/certs/fullchain.pem", KeyPath: "/certs/privkey.pem"}, Outbound: "direct"}
	until := time.Now().Add(time.Hour)
	return &config.Config{VPN: config.Policy{DefaultNode: "entry"}, Nodes: []config.Node{n}}, models.User{ID: 42, Status: models.UserStatusActive, ExpiresAt: &until, HY2Password: strings.Repeat("a", 64), SocksUsername: "s_42", SocksPassword: strings.Repeat("b", 64)}
}
func TestServerCredentialURIConsistency(t *testing.T) {
	c, u := fixture()
	spec, e := Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	users := spec.Inbounds[0].Users.([]node.User)
	uri, e := url.Parse(BuildHysteriaLink(c.Nodes[0], u.HY2Password))
	if e != nil {
		t.Fatal(e)
	}
	if users[0].Password != uri.User.Username() || uri.User.Username() != u.HY2Password {
		t.Fatal("authentication mismatch")
	}
	if _, hasPassword := uri.User.Password(); hasPassword || uri.Query().Get("insecure") != "" || uri.Host != "[2001:db8::1]:443" || uri.Query().Get("sni") != "vpn.example.com" {
		t.Fatal("unsafe or malformed URI")
	}
	if users[0].Name != "user_42" {
		t.Fatal("wrong Hysteria2 field")
	}
	b, _ := json.Marshal(spec)
	if strings.Contains(string(b), "username") || strings.Contains(string(b), "insecure") {
		t.Fatal("wrong inbound schema")
	}
}
func TestInactiveAndSelectedNode(t *testing.T) {
	c, u := fixture()
	for _, status := range []models.UserStatus{models.UserStatusPending, models.UserStatusRejected, models.UserStatusExpired, models.UserStatusSuspended, models.UserStatusRevoked} {
		u.Status = status
		spec, e := Generate(c, c.Nodes[0], []models.User{u}, time.Now())
		if e != nil || len(spec.Inbounds[0].Users.([]node.User)) != 0 {
			t.Fatal("disabled access present")
		}
	}
	u.Status = models.UserStatusActive
	u.NodeID = "other"
	spec, _ := Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	if len(spec.Inbounds[0].Users.([]node.User)) != 0 {
		t.Fatal("credential installed on unselected node")
	}
}
func TestCredentials(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		p, e := Credential()
		if e != nil || len(p) != 64 || seen[p] {
			t.Fatal("weak credential")
		}
		seen[p] = true
	}
	c, u := fixture()
	c.Nodes[0].Socks = config.Socks{Enabled: true, ListenAddress: "127.0.0.1", Port: 1080}
	spec, e := Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	s := spec.Inbounds[1].Users.([]node.SocksUser)[0]
	uri, _ := url.Parse(BuildSocksLink(c.Nodes[0], u.SocksUsername, u.SocksPassword))
	if uri.Query().Get("pass") != s.Password || uri.Query().Get("user") != s.Username {
		t.Fatal("SOCKS authentication mismatch")
	}
}
func TestSingBoxGeneratedCheck(t *testing.T) {
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Skip("set SINGBOX_TEST_BINARY to pinned executable for real config checks")
	}
	c, u := fixture()
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=vpn.example.com", "-addext", "subjectAltName=DNS:vpn.example.com", "-keyout", key, "-out", cert)
	if cmd.Run() != nil {
		t.Fatal("test certificate generation failed")
	}
	up := c.Nodes[0]
	up.ID = "upstream"
	up.Entry = false
	up.PublicAddress = "127.0.0.1"
	up.TransitPassword = strings.Repeat("c", 64)
	c.Nodes = append(c.Nodes, up)
	for _, mode := range []string{"direct", "upstream"} {
		c.Nodes[0].Outbound = mode
		spec, e := Generate(c, c.Nodes[0], []models.User{u}, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		spec.Inbounds[0].TLS.CertificatePath = cert
		spec.Inbounds[0].TLS.KeyPath = key
		b, _ := json.Marshal(spec)
		path := filepath.Join(dir, "config.json")
		os.WriteFile(path, b, 0600)
		if output, e := exec.Command(binary, "check", "-c", path).CombinedOutput(); e != nil {
			t.Fatalf("%s check: %s", mode, output)
		}
	}
}

func TestNoAnonymousSOCKSAndDisabledNode(t *testing.T) {
	c, u := fixture()
	n := c.Nodes[0]
	n.Socks = config.Socks{Enabled: true, ListenAddress: "127.0.0.1", Port: 1080}
	s, e := Generate(c, n, nil, time.Now())
	if e != nil || len(s.Inbounds) != 1 {
		t.Fatal("empty SOCKS user list would enable unauthenticated proxy")
	}
	n.Enabled = false
	s, e = Generate(c, n, []models.User{u}, time.Now())
	if e != nil || len(s.Inbounds[0].Users.([]node.User)) != 0 || len(s.Inbounds) != 1 {
		t.Fatal("disabled node retains access")
	}
}

func TestCredentialLabelsIndependent(t *testing.T) {
	g := NewKeyGenerator()
	for range 10 {
		user, password, e := g.NewSocksCredentials()
		if e != nil || len(user) != 26 || len(password) != 64 || strings.Contains(password, user[2:]) {
			t.Fatal("credential label exposes secret material")
		}
		hy, hyPassword, e := g.NewHysteriaCredentials()
		if e != nil || hyPassword == password || hy == user {
			t.Fatal("protocol credentials not independent")
		}
	}
}
