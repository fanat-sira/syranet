package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDistributedConfigEnvOverrides(t *testing.T) {
	// Read the real distributed YAML, including its relative template path.
	values := map[string]string{
		"DB_HOST": "postgres", "DB_PORT": "5432", "DB_NAME": "vpn_service",
		"DB_USER": "vpn_admin", "DB_PASSWORD": "db # ' : $ password",
		"USER_BOT_TOKEN": "123:dummy", "ADMIN_BOT_TOKEN": "456:dummy",
		"ADMIN_IDS": "123,456", "RU_SERVER_ADDRESS": "ru.example.invalid",
		"NL_SERVER_ADDRESS": "nl.example.invalid", "NL_PASSWORD": "nl # ' : $ password",
		"FALLBACK_HY2_PASSWORD": "", "FALLBACK_SOCKS_USERNAME": "", "FALLBACK_SOCKS_PASSWORD": "",
		"CLASH_API_URL":         "http://127.0.0.1:9090",
		"SINGBOX_TEMPLATE_PATH": "singbox_template.json",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Password != values["DB_PASSWORD"] || cfg.VPN.NLPassword != values["NL_PASSWORD"] {
		t.Fatal("environment credentials were changed during config loading")
	}
	if len(cfg.Bots.AdminIDs) != 2 || !cfg.IsAdmin(456) || cfg.IsAdmin(789) {
		t.Fatal("admin IDs were not applied correctly")
	}
	if _, err := os.Stat(cfg.VPN.SingBox.TemplatePath); err != nil {
		t.Fatalf("distributed template is not reachable: %v", err)
	}
	if cfg.VPN.SingBox.TemplatePath != filepath.Clean("../../configs/singbox_template.json") {
		t.Fatalf("unexpected template path: %s", cfg.VPN.SingBox.TemplatePath)
	}
	cfg.VPN.Fallback.SocksUsername = "legacy-user"
	if cfg.Validate() == nil {
		t.Fatal("unpaired SOCKS fallback must fail validation")
	}
}

func TestDSNRoundTrip(t *testing.T) {
	for _, host := range []string{"postgres", "127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Database: DatabaseConfig{
				Host: host, Port: 5432, Name: "vpn/db #1", User: "vpn user",
				Password: "space ' \\ # $ : @ ? / % & =", SSLMode: "disable",
				MaxOpenConns: 10, MinOpenConns: 2,
			}}
			parsed, err := pgxpool.ParseConfig(cfg.DSN())
			if err != nil {
				t.Fatal(err)
			}
			conn := parsed.ConnConfig
			if conn.Host != host || conn.Port != 5432 || conn.Database != cfg.Database.Name || conn.User != cfg.Database.User || conn.Password != cfg.Database.Password {
				t.Fatal("connection parameters did not survive DSN encoding")
			}
			if parsed.MaxConns != 10 || parsed.MinConns != 2 {
				t.Fatal("pool settings did not survive DSN encoding")
			}
		})
	}
}
