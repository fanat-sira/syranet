package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"vpn-service/internal/bot"
	"vpn-service/internal/config"
	"vpn-service/internal/database"
	"vpn-service/internal/service"
)

func main() {
	configPath := flag.String("config", envOrDefault("CONFIG_PATH", "configs/config.yaml"), "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}

	logger, err := newLogger(cfg.App.LogLevel, cfg.App.Env)
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	store, err := database.New(ctx, cfg.Database, logger.Named("database"))
	if err != nil {
		logger.Fatal("connect database", zap.Error(err))
	}
	defer store.Close()

	questionnaire := bot.NewQuestionnaireManager()
	keygen := service.NewKeyGenerator(cfg.VPN.RUAddress, cfg.VPN.HysteriaPort, cfg.VPN.SocksPort, cfg.VPN.HysteriaLinkName)
	configGenerator := service.NewConfigGenerator(cfg.VPN)
	singboxManager := service.NewSingBoxManager(cfg.VPN.SingBox, configGenerator, logger.Named("singbox"))

	syncAccess := func(syncCtx context.Context) error {
		users, err := store.ListConfigUsers(syncCtx)
		if err != nil {
			return err
		}
		return singboxManager.SyncUsers(syncCtx, users)
	}

	adminBot, err := bot.NewAdminBot(*cfg, store, syncAccess, logger)
	if err != nil {
		logger.Fatal("create admin bot", zap.Error(err))
	}

	userBot, err := bot.NewUserBot(*cfg, store, questionnaire, keygen, adminBot, logger)
	if err != nil {
		logger.Fatal("create user bot", zap.Error(err))
	}

	if err := syncAccess(ctx); err != nil {
		logger.Warn("initial sing-box sync failed", zap.Error(err))
	}

	monitor := service.NewClashMonitor(store, cfg.Monitoring, cfg.VPN.ConcurrentConnectionLimit, adminBot, logger.Named("monitor"))

	var wg sync.WaitGroup
	errCh := make(chan error, 4)

	run := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("component stopped with error", zap.String("component", name), zap.Error(err))
				select {
				case errCh <- err:
				default:
				}
				stop()
			}
		}()
	}

	run("user-bot", userBot.Run)
	run("admin-bot", adminBot.Run)

	wg.Add(1)
	go func() {
		defer wg.Done()
		monitor.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runHousekeeping(ctx, cfg, store, syncAccess, adminBot, logger.Named("housekeeping"))
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		logger.Error("received component error", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		logger.Warn("shutdown timeout exceeded")
	}

	logger.Info("syranet-vpn stopped")
}

func runHousekeeping(ctx context.Context, cfg *config.Config, store *database.Store, syncAccess func(context.Context) error, adminNotifier bot.AdminNotifier, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	seenReminder := make(map[int64]time.Time)

	check := func() {
		expired, err := store.ExpireUsers(ctx)
		if err != nil {
			logger.Warn("expire users", zap.Error(err))
		} else if expired > 0 {
			logger.Info("expired users updated", zap.Int64("count", expired))
			if err := syncAccess(ctx); err != nil {
				logger.Warn("sync access after expiration", zap.Error(err))
			}
		}

		users, err := store.ListUsersExpiringBefore(ctx, time.Now().Add(cfg.VPN.ReminderThreshold))
		if err != nil {
			logger.Warn("list expiring users", zap.Error(err))
			return
		}

		for _, user := range users {
			if user.ExpiresAt == nil || adminNotifier == nil {
				continue
			}
			if last, ok := seenReminder[user.ID]; ok && time.Since(last) < 24*time.Hour {
				continue
			}
			seenReminder[user.ID] = time.Now()

			text := fmt.Sprintf(
				"Subscription expiring soon\nUser ID: %d\nTelegram ID: %d\nUsername: @%s\nExpires at: %s",
				user.ID,
				user.TelegramID,
				empty(user.TelegramUsername, "unknown"),
				user.ExpiresAt.Format(time.RFC3339),
			)
			if err := adminNotifier.NotifyAdmins(ctx, text); err != nil {
				logger.Warn("notify admins about expiring user", zap.Int64("user_id", user.ID), zap.Error(err))
			}
		}
	}

	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func newLogger(level, env string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if env == "development" {
		cfg = zap.NewDevelopmentConfig()
	}

	parsedLevel := zapcore.InfoLevel
	if err := parsedLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	cfg.Level = zap.NewAtomicLevelAt(parsedLevel)
	return cfg.Build()
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func empty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
