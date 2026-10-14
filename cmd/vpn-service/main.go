package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/fanat-sira/syranet/internal/bot"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/node"
	"github.com/fanat-sira/syranet/internal/service"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(os.Args) > 1 && os.Args[1] == "node" {
		return runNode(ctx, os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "health" {
		return health()
	}
	flags := flag.NewFlagSet("syranet", flag.ContinueOnError)
	path := flags.String("config", env("CONFIG_PATH", "configs/config.yaml"), "platform YAML")
	check := flags.Bool("check", false, "validate without connecting or changing state")
	migrate := flags.Bool("migrate", false, "apply database migrations and exit")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if !*check && !*migrate {
		initial, _ := json.Marshal(struct {
			Ready bool      `json:"ready"`
			Time  time.Time `json:"time"`
		}{false, time.Now().UTC()})
		if node.AtomicWrite("/state/health.json", initial) != nil {
			return errors.New("management state directory is not writable")
		}
	}
	cfg, e := config.Load(*path)
	if e != nil {
		return e
	}
	if *check {
		return nil
	}
	logConfig := zap.NewProductionConfig()
	if e = logConfig.Level.UnmarshalText([]byte(cfg.App.LogLevel)); e != nil {
		return errors.New("invalid app.log_level")
	}
	logger, e := logConfig.Build()
	if e != nil {
		return errors.New("logger initialization failed")
	}
	defer logger.Sync()
	store, e := database.New(ctx, cfg.Database, logger)
	if e != nil {
		return errors.New("database unavailable; check DB settings")
	}
	defer store.Close()
	if e = store.Migrate(ctx); e != nil {
		return e
	}
	if *migrate {
		return nil
	}
	rec, e := service.NewReconciler(cfg, store)
	if e != nil {
		return e
	}
	admin, e := bot.NewAdminBot(*cfg, store, rec.Sync, logger)
	if e != nil {
		return e
	}
	user, e := bot.NewUserBot(*cfg, store, bot.NewQuestionnaireManager(), service.NewKeyGenerator(), admin, logger)
	if e != nil {
		return e
	}
	user.SetSync(rec.Sync)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); errs <- admin.Run(ctx) }()
	go func() { defer workers.Done(); errs <- user.Run(ctx) }()
	defer func() {
		cancel()
		done := make(chan struct{})
		go func() { workers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(cfg.App.ShutdownTimeout):
		}
	}()
	ticker := time.NewTicker(cfg.App.ReconcileInterval)
	defer ticker.Stop()
	lastAlert := time.Time{}
	reminders := map[int64]time.Time{}
	reconcile := func() {
		_, expireErr := store.ExpireUsers(ctx)
		syncErr := rec.Sync(ctx)
		ready := expireErr == nil && syncErr == nil
		state, _ := json.Marshal(struct {
			Ready      bool      `json:"ready"`
			Time       time.Time `json:"time"`
			ValidUntil time.Time `json:"valid_until"`
		}{ready, time.Now().UTC(), time.Now().UTC().Add(cfg.App.ReconcileInterval + 2*time.Minute)})
		_ = node.AtomicWrite("/state/health.json", state)
		if !ready && time.Since(lastAlert) > 15*time.Minute {
			logger.Warn("infrastructure sync failed; desired access pending")
			_ = admin.NotifyAdmins(ctx, "Infrastructure error: access changes pending. Check /nodes and node TLS/connectivity.")
			lastAlert = time.Now()
		}
		expiring, e := store.ListUsersExpiringBefore(ctx, time.Now().Add(cfg.VPN.ReminderThreshold))
		if e == nil {
			for _, u := range expiring {
				if time.Since(reminders[u.ID]) > 24*time.Hour {
					if admin.NotifyAdmins(ctx, fmt.Sprintf("Subscription expiring: user %d. /info %d", u.ID, u.ID)) == nil {
						reminders[u.ID] = time.Now()
					}
				}
			}
		}
	}
	reconcile()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-errs:
			cancel()
			return errors.New("Telegram component stopped")
		case <-ticker.C:
			reconcile()
		}
	}
}
func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func health() error {
	if os.Getenv("SYRANET_ROLE") == "node" {
		c, e := node.NewClient(env("NODE_HEALTH_URL", "http://127.0.0.1:8080"), os.Getenv("NODE_TOKEN"), os.Getenv("NODE_HEALTH_CA"))
		if e != nil {
			return e
		}
		s, e := c.Status(context.Background())
		if e != nil || !s.Running || !s.TLSReady || s.Error != "" {
			return errors.New("node not ready")
		}
		return nil
	}
	b, e := os.ReadFile("/state/health.json")
	if e != nil {
		return errors.New("manager not ready")
	}
	var s struct {
		Ready      bool
		Time       time.Time
		ValidUntil time.Time `json:"valid_until"`
	}
	if json.Unmarshal(b, &s) != nil || !s.Ready || !time.Now().Before(s.ValidUntil) {
		return errors.New("manager reconciliation unhealthy")
	}
	return nil
}
func runNode(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("node", flag.ContinueOnError)
	listen := flags.String("listen", env("NODE_LISTEN", ":8080"), "API listener")
	dir := flags.String("state", env("NODE_STATE", "/state"), "private state directory")
	binary := flags.String("binary", env("SINGBOX_BINARY", "/usr/local/bin/sing-box"), "pinned sing-box executable")
	if e := flags.Parse(args); e != nil {
		return e
	}
	token := os.Getenv("NODE_TOKEN")
	if len(token) < 32 {
		return errors.New("NODE_TOKEN must contain at least 32 characters")
	}
	version, e := exec.Command(*binary, "version").Output()
	if e != nil || !strings.Contains(string(version), "sing-box version "+config.SingBoxVersion+"\n") {
		return errors.New("node requires sing-box " + config.SingBoxVersion)
	}
	sup := node.New(*dir, &node.Process{Binary: *binary})
	defer sup.Close()
	if e = sup.Restore(ctx); e != nil {
		fmt.Fprintln(os.Stderr, "persisted configuration could not start; awaiting reconciliation")
	}
	srv := &http.Server{Addr: *listen, Handler: node.Handler(sup, token), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	cert, key := os.Getenv("NODE_API_TLS_CERT"), os.Getenv("NODE_API_TLS_KEY")
	if cert != "" && key != "" {
		if _, e = tls.LoadX509KeyPair(cert, key); e != nil {
			return errors.New("node API TLS certificate/key cannot be loaded")
		}
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			pair, e := tls.LoadX509KeyPair(cert, key)
			return &pair, e
		}}
		e = srv.ListenAndServeTLS("", "")
	} else if cert != "" || key != "" {
		return errors.New("both NODE_API_TLS_CERT and NODE_API_TLS_KEY are required")
	} else {
		if os.Getenv("NODE_ALLOW_PLAINTEXT") != "true" {
			return errors.New("node API requires TLS; NODE_ALLOW_PLAINTEXT only for an unpublished internal Compose network")
		}
		e = srv.ListenAndServe()
	}
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return errors.New("node API cannot listen; inspect ports and TLS files")
}
