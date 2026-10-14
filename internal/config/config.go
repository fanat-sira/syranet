package config

import (
	"errors"
	"fmt"
	"go.yaml.in/yaml/v3"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const SingBoxVersion = "1.14.3"

type Config struct {
	App      AppConfig      `yaml:"app"`
	Database DatabaseConfig `yaml:"-"`
	Bots     BotsConfig     `yaml:"-"`
	VPN      Policy         `yaml:"policy"`
	Nodes    []Node         `yaml:"nodes"`
}
type AppConfig struct {
	LogLevel          string        `yaml:"log_level"`
	ReconcileInterval time.Duration `yaml:"reconcile_interval"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
}
type DatabaseConfig struct {
	Host                          string
	Port                          int
	Name, User, Password, SSLMode string
	MaxOpenConns, MinOpenConns    int32
	MaxConnLifetime               time.Duration
}
type BotsConfig struct {
	UserToken, AdminToken string
	AdminIDs              []int64
	PollingTimeout        int
}
type Policy struct {
	SubscriptionDays  int           `yaml:"subscription_days"`
	ReminderThreshold time.Duration `yaml:"reminder_threshold"`
	DefaultNode       string        `yaml:"default_node"`
}
type TLS struct {
	ServerName      string `yaml:"server_name"`
	CertificatePath string `yaml:"certificate_path"`
	KeyPath         string `yaml:"key_path"`
	CAPath          string `yaml:"ca_path,omitempty"`
}
type Node struct {
	ID                 string `yaml:"id"`
	Name               string `yaml:"name"`
	Enabled            bool   `yaml:"enabled"`
	Entry              bool   `yaml:"entry"`
	PublicAddress      string `yaml:"public_address"`
	ListenAddress      string `yaml:"listen_address"`
	Port               int    `yaml:"port"`
	TLS                TLS    `yaml:"tls"`
	Agent              Agent  `yaml:"agent"`
	Outbound           string `yaml:"outbound"`
	TransitPasswordEnv string `yaml:"transit_password_env,omitempty"`
	TransitPassword    string `yaml:"-"`
	Socks              Socks  `yaml:"socks"`
}
type Agent struct {
	URL      string `yaml:"url"`
	TokenEnv string `yaml:"token_env"`
	CAPath   string `yaml:"ca_path,omitempty"`
	Token    string `yaml:"-"`
}
type Socks struct {
	Enabled       bool   `yaml:"enabled"`
	ListenAddress string `yaml:"listen_address"`
	Port          int    `yaml:"port"`
	AllowPublic   bool   `yaml:"allow_public"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var secretEnv = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
var botToken = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)
var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

func Load(path string) (*Config, error) {
	c := &Config{App: AppConfig{LogLevel: "info", ReconcileInterval: 15 * time.Second, ShutdownTimeout: 15 * time.Second}, VPN: Policy{SubscriptionDays: 30, ReminderThreshold: 72 * time.Hour}, Database: DatabaseConfig{Host: "postgres", Port: 5432, Name: "syranet", User: "syranet", SSLMode: "disable", MaxOpenConns: 10, MinOpenConns: 1, MaxConnLifetime: 30 * time.Minute}, Bots: BotsConfig{PollingTimeout: 30}}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read CONFIG_PATH")
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err = dec.Decode(c); err != nil {
		return nil, errors.New("invalid configuration YAML or unknown setting")
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("configuration must contain one YAML document")
	}
	for key, target := range map[string]*string{"DB_HOST": &c.Database.Host, "DB_NAME": &c.Database.Name, "DB_USER": &c.Database.User, "DB_PASSWORD": &c.Database.Password, "DB_SSLMODE": &c.Database.SSLMode, "USER_BOT_TOKEN": &c.Bots.UserToken, "ADMIN_BOT_TOKEN": &c.Bots.AdminToken} {
		if v, ok := os.LookupEnv(key); ok {
			*target = v
		}
	}
	if v, ok := os.LookupEnv("DB_PORT"); ok {
		c.Database.Port, err = strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("invalid DB_PORT")
		}
	}
	for key, target := range map[string]*int{"BOT_POLLING_TIMEOUT": &c.Bots.PollingTimeout} {
		if v, ok := os.LookupEnv(key); ok {
			*target, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid %s", key)
			}
		}
	}
	for _, v := range strings.Split(os.Getenv("ADMIN_IDS"), ",") {
		id, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if e != nil || id <= 0 {
			return nil, errors.New("invalid ADMIN_IDS")
		}
		c.Bots.AdminIDs = append(c.Bots.AdminIDs, id)
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		n.Agent.Token = os.Getenv(n.Agent.TokenEnv)
		n.TransitPassword = os.Getenv(n.TransitPasswordEnv)
	}
	if value, ok := os.LookupEnv("HYSTERIA_PORT"); ok {
		published, parseErr := strconv.Atoi(value)
		if parseErr != nil || !port(published) {
			return nil, errors.New("invalid HYSTERIA_PORT")
		}
		for _, n := range c.Nodes {
			if strings.HasPrefix(n.Agent.URL, "http://node:") && n.Port != published {
				return nil, errors.New("HYSTERIA_PORT must match the local node.port")
			}
		}
	}
	if err = c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
func validAddress(s string) bool {
	if ip := net.ParseIP(s); ip != nil {
		return !ip.IsUnspecified() && !ip.IsMulticast()
	}
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) > 63 || !hostname.MatchString(label) || strings.Contains(label, ".") {
			return false
		}
	}
	return true
}
func port(p int) bool { return p > 0 && p <= 65535 }
func (c *Config) Validate() error {
	var p []string
	if !validAddress(c.Database.Host) || !port(c.Database.Port) || c.Database.Name == "" || c.Database.User == "" || c.Database.Password == "" {
		p = append(p, "invalid DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASSWORD")
	}
	if c.Database.SSLMode != "disable" && c.Database.SSLMode != "verify-full" {
		p = append(p, "DB_SSLMODE must be disable (internal network) or verify-full")
	}
	if !botToken.MatchString(c.Bots.UserToken) || !botToken.MatchString(c.Bots.AdminToken) || c.Bots.UserToken == c.Bots.AdminToken || len(c.Bots.AdminIDs) == 0 || c.Bots.PollingTimeout < 1 || c.Bots.PollingTimeout > 60 {
		p = append(p, "invalid USER_BOT_TOKEN/ADMIN_BOT_TOKEN/ADMIN_IDS/BOT_POLLING_TIMEOUT")
	}
	for _, id := range c.Bots.AdminIDs {
		if id <= 0 {
			p = append(p, "invalid ADMIN_IDS")
		}
	}
	if c.App.ReconcileInterval < time.Second || c.App.ShutdownTimeout < time.Second || c.VPN.SubscriptionDays < 1 || c.VPN.SubscriptionDays > 3650 || c.VPN.ReminderThreshold <= 0 {
		p = append(p, "invalid policy or app intervals")
	}
	if c.App.LogLevel != "info" && c.App.LogLevel != "warn" && c.App.LogLevel != "error" {
		p = append(p, "app.log_level must be info, warn or error")
	}
	nodes := map[string]Node{}
	agents := map[string]bool{}
	for _, n := range c.Nodes {
		prefix := "node: "
		if identifier.MatchString(n.ID) {
			prefix = "node " + n.ID + ": "
		}
		if len(n.Name) < 1 || len(n.Name) > 64 || strings.ContainsAny(n.Name, "\n\r\t") {
			p = append(p, prefix+"invalid name")
		}
		if !secretEnv.MatchString(n.Agent.TokenEnv) || (n.TransitPasswordEnv != "" && !secretEnv.MatchString(n.TransitPasswordEnv)) {
			p = append(p, prefix+"invalid secret environment key")
		}
		if agents[strings.TrimRight(n.Agent.URL, "/")] {
			p = append(p, prefix+"agent.url must be unique per node")
		}
		agents[strings.TrimRight(n.Agent.URL, "/")] = true
		if n.TLS.CAPath != "" && n.TLS.CAPath != "/certs/ca.pem" {
			p = append(p, prefix+"tls.ca_path must be /certs/ca.pem")
		}
		if !identifier.MatchString(n.ID) || nodes[n.ID].ID != "" {
			p = append(p, "invalid or duplicate node.id")
		}
		nodes[n.ID] = n
		if !validAddress(n.PublicAddress) || net.ParseIP(n.ListenAddress) == nil || !port(n.Port) {
			p = append(p, prefix+"invalid address or port")
		}
		if !validAddress(n.TLS.ServerName) || n.TLS.CertificatePath != "/certs/fullchain.pem" || n.TLS.KeyPath != "/certs/privkey.pem" {
			p = append(p, prefix+"tls requires server_name and /certs/fullchain.pem, /certs/privkey.pem")
		}
		u, e := url.Parse(n.Agent.URL)
		if e == nil && u.Port() != "" {
			pnum, err := strconv.Atoi(u.Port())
			if err != nil || !port(pnum) {
				p = append(p, prefix+"invalid agent.url port")
			}
		}
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			p = append(p, prefix+"invalid agent.url")
		} else if u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "node") {
			p = append(p, prefix+"remote agent.url requires HTTPS; HTTP only allowed for the private Compose service node")
		}
		if len(n.Agent.Token) < 32 {
			p = append(p, prefix+"agent token missing or too short; check agent.token_env")
		}
		if n.TransitPasswordEnv != "" && len(n.TransitPassword) < 32 {
			p = append(p, prefix+"transit password missing or too short")
		}
		if n.Outbound == "" {
			p = append(p, prefix+"outbound must be direct or a node ID")
		}
		if n.Socks.Enabled {
			ip := net.ParseIP(n.Socks.ListenAddress)
			if ip == nil || !port(n.Socks.Port) || (!ip.IsLoopback() && !ip.IsPrivate() && !n.Socks.AllowPublic) {
				p = append(p, prefix+"SOCKS requires a private listener or explicit allow_public")
			}
		}
	}
	def := nodes[c.VPN.DefaultNode]
	if !def.Enabled || !def.Entry {
		p = append(p, "policy.default_node must be an enabled entry node")
	}
	for _, n := range c.Nodes {
		if !n.Enabled {
			continue
		}
		seen := map[string]bool{}
		cur := n
		for cur.Outbound != "direct" {
			if seen[cur.ID] {
				p = append(p, "topology contains a cycle")
				break
			}
			seen[cur.ID] = true
			next, ok := nodes[cur.Outbound]
			if !ok || !next.Enabled || next.TransitPasswordEnv == "" {
				p = append(p, "outbound requires an enabled node with transit_password_env")
				break
			}
			cur = next
		}
	}
	if len(p) > 0 {
		return errors.New(strings.Join(p, "; "))
	}
	return nil
}
func (c *Config) DSN() string {
	q := url.Values{"sslmode": {c.Database.SSLMode}}
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(c.Database.User, c.Database.Password), Host: net.JoinHostPort(c.Database.Host, strconv.Itoa(c.Database.Port)), Path: "/" + c.Database.Name, RawQuery: q.Encode()}
	return u.String()
}
func (c *Config) IsAdmin(id int64) bool {
	for _, a := range c.Bots.AdminIDs {
		if a == id {
			return true
		}
	}
	return false
}
func (c *Config) Node(id string) (Node, bool) {
	if id == "" {
		id = c.VPN.DefaultNode
	}
	for _, n := range c.Nodes {
		if n.ID == id && n.Enabled {
			return n, true
		}
	}
	return Node{}, false
}
func (c *Config) Entries() []Node {
	var r []Node
	for _, n := range c.Nodes {
		if n.Enabled && n.Entry {
			r = append(r, n)
		}
	}
	return r
}
