package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Database   DatabaseConfig   `yaml:"database"`
	Bots       BotsConfig       `yaml:"bots"`
	VPN        VPNConfig        `yaml:"vpn"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
}

type AppConfig struct {
	Name            string        `yaml:"name"`
	Env             string        `yaml:"env"`
	LogLevel        string        `yaml:"log_level"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	Name            string        `yaml:"name"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	SSLMode         string        `yaml:"ssl_mode"`
	MaxOpenConns    int32         `yaml:"max_open_conns"`
	MinOpenConns    int32         `yaml:"min_open_conns"`
	MaxConnLifetime time.Duration `yaml:"max_conn_lifetime"`
}

type BotsConfig struct {
	UserToken      string  `yaml:"user_token"`
	AdminToken     string  `yaml:"admin_token"`
	AdminIDs       []int64 `yaml:"admin_ids"`
	PollingTimeout int     `yaml:"polling_timeout"`
}

type VPNConfig struct {
	RUAddress                 string         `yaml:"ru_server_address"`
	NLAddress                 string         `yaml:"nl_server_address"`
	NLPassword                string         `yaml:"nl_password"`
	HysteriaPort              int            `yaml:"hysteria_port"`
	SocksPort                 int            `yaml:"socks_port"`
	SubscriptionDays          int            `yaml:"subscription_days"`
	HysteriaLinkName          string         `yaml:"hysteria_link_name"`
	SocksLinkName             string         `yaml:"socks_link_name"`
	ConcurrentConnectionLimit int            `yaml:"concurrent_connection_limit"`
	ReminderThreshold         time.Duration  `yaml:"reminder_threshold"`
	SingBox                   SingBoxConfig  `yaml:"singbox"`
	Fallback                  FallbackConfig `yaml:"fallback"`
}

type SingBoxConfig struct {
	TemplatePath string        `yaml:"template_path"`
	ConfigPath   string        `yaml:"config_path"`
	BinaryPath   string        `yaml:"binary_path"`
	ConfigDir    string        `yaml:"config_dir"`
	UseSystemd   bool          `yaml:"use_systemd"`
	ServiceName  string        `yaml:"service_name"`
	PIDFilePath  string        `yaml:"pid_file_path"`
	ReloadSignal string        `yaml:"reload_signal"`
	CheckTimeout time.Duration `yaml:"check_timeout"`
}

type FallbackConfig struct {
	HY2Password   string `yaml:"hy2_password"`
	SocksUsername string `yaml:"socks_username"`
	SocksPassword string `yaml:"socks_password"`
}

type MonitoringConfig struct {
	Enabled                  bool          `yaml:"enabled"`
	ClashAPIURL              string        `yaml:"clash_api_url"`
	PollInterval             time.Duration `yaml:"poll_interval"`
	DisconnectExtraSessions  bool          `yaml:"disconnect_extra_sessions"`
	AdminNotificationEnabled bool          `yaml:"admin_notification_enabled"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}

	applyEnv(cfg)
	cfg.normalize(path)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	var problems []string

	if c.Database.Host == "" {
		problems = append(problems, "database.host is required")
	}
	if c.Database.Port == 0 {
		problems = append(problems, "database.port is required")
	}
	if c.Database.Name == "" {
		problems = append(problems, "database.name is required")
	}
	if c.Database.User == "" {
		problems = append(problems, "database.user is required")
	}
	if c.Database.Password == "" {
		problems = append(problems, "database.password is required")
	}
	if c.Bots.UserToken == "" {
		problems = append(problems, "bots.user_token is required")
	}
	if c.Bots.AdminToken == "" {
		problems = append(problems, "bots.admin_token is required")
	}
	if len(c.Bots.AdminIDs) == 0 {
		problems = append(problems, "bots.admin_ids is required")
	}
	if c.VPN.RUAddress == "" {
		problems = append(problems, "vpn.ru_server_address is required")
	}
	if c.VPN.NLAddress == "" {
		problems = append(problems, "vpn.nl_server_address is required")
	}
	if c.VPN.NLPassword == "" {
		problems = append(problems, "vpn.nl_password is required")
	}
	if c.VPN.SingBox.TemplatePath == "" {
		problems = append(problems, "singbox.template_path is required")
	}
	if c.VPN.SingBox.ConfigPath == "" {
		problems = append(problems, "singbox.config_path is required")
	}
	if c.VPN.SubscriptionDays <= 0 {
		problems = append(problems, "vpn.subscription_days must be greater than zero")
	}
	if c.VPN.ConcurrentConnectionLimit <= 0 {
		problems = append(problems, "vpn.concurrent_connection_limit must be greater than zero")
	}
	if c.Monitoring.Enabled && c.Monitoring.ClashAPIURL == "" {
		problems = append(problems, "monitoring.clash_api_url is required when monitoring is enabled")
	}
	if c.VPN.SingBox.UseSystemd && c.VPN.SingBox.ServiceName == "" {
		problems = append(problems, "vpn.singbox.service_name is required when use_systemd is enabled")
	}
	if (c.VPN.Fallback.SocksUsername == "") != (c.VPN.Fallback.SocksPassword == "") {
		problems = append(problems, "vpn.fallback.socks_username and vpn.fallback.socks_password must be set together")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (c *Config) DSN() string {
	params := url.Values{}
	params.Set("dbname", c.Database.Name)
	params.Set("sslmode", c.Database.SSLMode)
	params.Set("pool_max_conns", strconv.Itoa(int(c.Database.MaxOpenConns)))
	params.Set("pool_min_conns", strconv.Itoa(int(c.Database.MinOpenConns)))
	dsn := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(c.Database.User, c.Database.Password),
		Host:     net.JoinHostPort(c.Database.Host, strconv.Itoa(c.Database.Port)),
		RawQuery: params.Encode(),
	}
	return dsn.String()
}

func (c *Config) IsAdmin(telegramID int64) bool {
	for _, id := range c.Bots.AdminIDs {
		if id == telegramID {
			return true
		}
	}
	return false
}

func (c *Config) normalize(configPath string) {
	baseDir := "."
	if configPath != "" {
		baseDir = filepath.Dir(configPath)
	}

	if !filepath.IsAbs(c.VPN.SingBox.TemplatePath) {
		c.VPN.SingBox.TemplatePath = filepath.Clean(filepath.Join(baseDir, c.VPN.SingBox.TemplatePath))
	}
	if c.App.Name == "" {
		c.App.Name = "syranet-vpn"
	}
	if c.App.Env == "" {
		c.App.Env = "development"
	}
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}
	if c.VPN.SingBox.BinaryPath == "" {
		c.VPN.SingBox.BinaryPath = "/usr/bin/sing-box"
	}
	if c.VPN.SingBox.ConfigDir == "" && c.VPN.SingBox.ConfigPath != "" {
		c.VPN.SingBox.ConfigDir = filepath.Dir(c.VPN.SingBox.ConfigPath)
	}
	if c.VPN.SingBox.ServiceName == "" {
		c.VPN.SingBox.ServiceName = "sing-box"
	}
	if c.VPN.SingBox.ReloadSignal == "" {
		c.VPN.SingBox.ReloadSignal = "HUP"
	}
	if c.Monitoring.ClashAPIURL == "" {
		c.Monitoring.ClashAPIURL = "http://127.0.0.1:9090"
	}
}

func defaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:            "syranet-vpn",
			Env:             "development",
			LogLevel:        "info",
			ShutdownTimeout: 15 * time.Second,
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Name:            "vpn_service",
			User:            "vpn_admin",
			Password:        "secure_password",
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MinOpenConns:    2,
			MaxConnLifetime: 30 * time.Minute,
		},
		Bots: BotsConfig{
			PollingTimeout: 30,
		},
		VPN: VPNConfig{
			HysteriaPort:              443,
			SocksPort:                 8443,
			SubscriptionDays:          30,
			HysteriaLinkName:          "Syranet VPN",
			SocksLinkName:             "Syranet VPN SOCKS",
			ConcurrentConnectionLimit: 1,
			ReminderThreshold:         72 * time.Hour,
			SingBox: SingBoxConfig{
				TemplatePath: "configs/singbox_template.json",
				ConfigPath:   "/etc/sing-box/config.json",
				BinaryPath:   "/usr/bin/sing-box",
				ConfigDir:    "/etc/sing-box",
				UseSystemd:   true,
				ServiceName:  "sing-box",
				ReloadSignal: "HUP",
				CheckTimeout: 10 * time.Second,
			},
		},
		Monitoring: MonitoringConfig{
			Enabled:                  true,
			ClashAPIURL:              "http://127.0.0.1:9090",
			PollInterval:             30 * time.Second,
			DisconnectExtraSessions:  false,
			AdminNotificationEnabled: true,
		},
	}
}

func applyEnv(cfg *Config) {
	setString(&cfg.App.Env, "APP_ENV")
	setString(&cfg.App.LogLevel, "LOG_LEVEL")
	setDuration(&cfg.App.ShutdownTimeout, "APP_SHUTDOWN_TIMEOUT")

	setString(&cfg.Database.Host, "DB_HOST")
	setInt(&cfg.Database.Port, "DB_PORT")
	setString(&cfg.Database.Name, "DB_NAME")
	setString(&cfg.Database.User, "DB_USER")
	setString(&cfg.Database.Password, "DB_PASSWORD")
	setString(&cfg.Database.SSLMode, "DB_SSLMODE")
	setInt32(&cfg.Database.MaxOpenConns, "DB_MAX_OPEN_CONNS")
	setInt32(&cfg.Database.MinOpenConns, "DB_MIN_OPEN_CONNS")
	setDuration(&cfg.Database.MaxConnLifetime, "DB_MAX_CONN_LIFETIME")

	setString(&cfg.Bots.UserToken, "USER_BOT_TOKEN")
	setString(&cfg.Bots.AdminToken, "ADMIN_BOT_TOKEN")
	setIntSlice(&cfg.Bots.AdminIDs, "ADMIN_IDS")
	setInt(&cfg.Bots.PollingTimeout, "BOT_POLLING_TIMEOUT")

	setString(&cfg.VPN.RUAddress, "RU_SERVER_ADDRESS")
	setString(&cfg.VPN.NLAddress, "NL_SERVER_ADDRESS")
	setString(&cfg.VPN.NLPassword, "NL_PASSWORD")
	setInt(&cfg.VPN.HysteriaPort, "HYSTERIA_PORT")
	setInt(&cfg.VPN.SocksPort, "SOCKS_PORT")
	setInt(&cfg.VPN.SubscriptionDays, "SUBSCRIPTION_DAYS")
	setString(&cfg.VPN.HysteriaLinkName, "HYSTERIA_LINK_NAME")
	setString(&cfg.VPN.SocksLinkName, "SOCKS_LINK_NAME")
	setInt(&cfg.VPN.ConcurrentConnectionLimit, "CONCURRENT_CONNECTION_LIMIT")
	setDuration(&cfg.VPN.ReminderThreshold, "REMINDER_THRESHOLD")

	setString(&cfg.VPN.SingBox.TemplatePath, "SINGBOX_TEMPLATE_PATH")
	setString(&cfg.VPN.SingBox.ConfigPath, "SINGBOX_CONFIG_PATH")
	setString(&cfg.VPN.SingBox.BinaryPath, "SINGBOX_BINARY_PATH")
	setString(&cfg.VPN.SingBox.ConfigDir, "SINGBOX_CONFIG_DIR")
	setBool(&cfg.VPN.SingBox.UseSystemd, "SINGBOX_USE_SYSTEMD")
	setString(&cfg.VPN.SingBox.ServiceName, "SINGBOX_SERVICE_NAME")
	setString(&cfg.VPN.SingBox.PIDFilePath, "SINGBOX_PID_FILE")
	setString(&cfg.VPN.SingBox.ReloadSignal, "SINGBOX_RELOAD_SIGNAL")
	setDuration(&cfg.VPN.SingBox.CheckTimeout, "SINGBOX_CHECK_TIMEOUT")

	setString(&cfg.VPN.Fallback.HY2Password, "FALLBACK_HY2_PASSWORD")
	setString(&cfg.VPN.Fallback.SocksUsername, "FALLBACK_SOCKS_USERNAME")
	setString(&cfg.VPN.Fallback.SocksPassword, "FALLBACK_SOCKS_PASSWORD")

	setBool(&cfg.Monitoring.Enabled, "MONITORING_ENABLED")
	setString(&cfg.Monitoring.ClashAPIURL, "CLASH_API_URL")
	setDuration(&cfg.Monitoring.PollInterval, "CLASH_API_POLL_INTERVAL")
	setBool(&cfg.Monitoring.DisconnectExtraSessions, "DISCONNECT_EXTRA_SESSIONS")
	setBool(&cfg.Monitoring.AdminNotificationEnabled, "ADMIN_NOTIFICATION_ENABLED")
}

func setString(target *string, key string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = strings.TrimSpace(value)
	}
}

func setInt(target *int, key string) {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			*target = parsed
		}
	}
}

func setInt32(target *int32, key string) {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
		if err == nil {
			*target = int32(parsed)
		}
	}
}

func setBool(target *bool, key string) {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			*target = parsed
		}
	}
}

func setDuration(target *time.Duration, key string) {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err == nil {
			*target = parsed
		}
	}
}

func setIntSlice(target *[]int64, key string) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return
	}

	parts := strings.Split(value, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			result = append(result, parsed)
		}
	}
	if len(result) > 0 {
		*target = result
	}
}
