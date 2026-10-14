package config

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func environment(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{"DB_PASSWORD": "db # ' : $ password", "USER_BOT_TOKEN": "123:dummy", "ADMIN_BOT_TOKEN": "456:dummy", "ADMIN_IDS": "123,456",
		"HYSTERIA_PORT": "443", "ENTRY_AGENT_TOKEN": strings.Repeat("a", 64), "UPSTREAM_AGENT_TOKEN": strings.Repeat("b", 64), "UPSTREAM_TRANSIT_PASSWORD": strings.Repeat("c", 64)} {
		t.Setenv(k, v)
	}
}
func TestExamples(t *testing.T) {
	environment(t)
	for _, f := range []string{"config.yaml", "single-node.yaml", "multi-node.yaml"} {
		c, e := Load("../../configs/" + f)
		if e != nil {
			t.Fatalf("%s: %v", f, e)
		}
		if c.Database.Password != "db # ' : $ password" || !c.IsAdmin(456) || c.IsAdmin(789) {
			t.Fatal("env or auth mismatch")
		}
	}
}
func TestInvalidEnvFails(t *testing.T) {
	environment(t)
	for _, key := range []string{"DB_PORT", "ADMIN_IDS", "BOT_POLLING_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "secret_invalid")
			_, e := Load("../../configs/config.yaml")
			if e == nil || strings.Contains(e.Error(), "secret_invalid") {
				t.Fatal("invalid env accepted or disclosed")
			}
		})
	}
}
func TestTopologyValidation(t *testing.T) {
	environment(t)
	c, e := Load("../../configs/multi-node.yaml")
	if e != nil {
		t.Fatal(e)
	}
	c.Nodes[1].Outbound = "entry"
	c.Nodes[0].TransitPasswordEnv = "TRANSIT"
	c.Nodes[0].TransitPassword = strings.Repeat("c", 64)
	if c.Validate() == nil {
		t.Fatal("cycle accepted")
	}
	c.Nodes[1].Outbound = "direct"
	c.Nodes[0].Port = 65536
	if c.Validate() == nil {
		t.Fatal("invalid port accepted")
	}
	c.Nodes[0].Port = 443
	c.Nodes[0].Agent.URL = "http://public.example.com:8080"
	if c.Validate() == nil {
		t.Fatal("plaintext remote agent accepted")
	}
}
func TestUnknownYAML(t *testing.T) {
	environment(t)
	p := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(p, []byte("database:\n  password: secret\n"), 0600)
	_, e := Load(p)
	if e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal("unknown config accepted or disclosed")
	}
}
func TestDSNRoundTrip(t *testing.T) {
	for _, host := range []string{"postgres", "127.0.0.1", "::1"} {
		c := Config{Database: DatabaseConfig{Host: host, Port: 5432, Name: "vpn/db #1", User: "vpn user", Password: "space ' \\ # $ : @ ? / % & =", SSLMode: "disable"}}
		p, e := pgxpool.ParseConfig(c.DSN())
		if e != nil {
			t.Fatal(e)
		}
		if p.ConnConfig.Host != host || p.ConnConfig.Password != c.Database.Password || p.ConnConfig.Database != c.Database.Name {
			t.Fatalf("DSN mismatch host=%q expected=%q db=%q expected=%q passwordEqual=%t", p.ConnConfig.Host, host, p.ConnConfig.Database, c.Database.Name, p.ConnConfig.Password == c.Database.Password)
		}
	}
}

func TestPublishedPortMismatch(t *testing.T) {
	environment(t)
	t.Setenv("HYSTERIA_PORT", "8443")
	_, e := Load("../../configs/single-node.yaml")
	if e == nil {
		t.Fatal("published UDP port differs from node listener")
	}
}
