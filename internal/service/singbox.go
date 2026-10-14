package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"vpn-service/internal/config"
	"vpn-service/internal/database"
	"vpn-service/internal/models"
)

type AdminNotifier interface {
	NotifyAdmins(ctx context.Context, text string) error
}

type SingBoxManager struct {
	cfg       config.SingBoxConfig
	generator *ConfigGenerator
	logger    *zap.Logger
}

func NewSingBoxManager(cfg config.SingBoxConfig, generator *ConfigGenerator, logger *zap.Logger) *SingBoxManager {
	return &SingBoxManager{cfg: cfg, generator: generator, logger: logger}
}

func (m *SingBoxManager) SyncUsers(ctx context.Context, users []models.User) error {
	payload, err := m.generator.Generate(users)
	if err != nil {
		return err
	}
	if err := m.Validate(ctx, payload); err != nil {
		return err
	}
	if err := m.writeAtomically(payload); err != nil {
		return err
	}
	if err := m.Reload(); err != nil {
		return err
	}
	m.logger.Info("sing-box configuration synced", zap.Int("users", len(users)))
	return nil
}

func (m *SingBoxManager) Validate(ctx context.Context, payload []byte) error {
	configDir := m.cfg.ConfigDir
	if configDir == "" {
		configDir = filepath.Dir(m.cfg.ConfigPath)
	}

	tmpFile, err := os.CreateTemp(configDir, "singbox-check-*.json")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, m.cfg.CheckTimeout)
	defer cancel()

	args := []string{"check"}
	if configDir != "" {
		args = append(args, "-C", configDir)
	}
	args = append(args, "-c", tmpFile.Name())

	cmd := exec.CommandContext(checkCtx, m.cfg.BinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sing-box config validation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *SingBoxManager) Reload() error {
	if m.cfg.UseSystemd {
		serviceName := m.cfg.ServiceName
		if serviceName == "" {
			serviceName = "sing-box"
		}
		cmd := exec.Command("systemctl", "reload", serviceName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl reload failed: %w, output: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}

	pid, err := m.resolvePID()
	if err != nil {
		return err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find sing-box process: %w", err)
	}

	signal := syscall.SIGHUP
	if strings.EqualFold(m.cfg.ReloadSignal, "TERM") {
		signal = syscall.SIGTERM
	}
	if err := process.Signal(signal); err != nil {
		return fmt.Errorf("signal sing-box process: %w", err)
	}
	return nil
}

func (m *SingBoxManager) writeAtomically(payload []byte) error {
	dir := filepath.Dir(m.cfg.ConfigPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sing-box config dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "config-*.json")
	if err != nil {
		return fmt.Errorf("create destination temp config: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, bytes.NewReader(payload)); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write destination temp config: %w", err)
	}
	if err := tmpFile.Chmod(0o640); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod destination temp config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close destination temp config: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), m.cfg.ConfigPath); err != nil {
		return fmt.Errorf("replace sing-box config: %w", err)
	}
	return nil
}

func (m *SingBoxManager) resolvePID() (int, error) {
	if m.cfg.PIDFilePath != "" {
		data, err := os.ReadFile(m.cfg.PIDFilePath)
		if err != nil {
			return 0, fmt.Errorf("read sing-box pid file: %w", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return 0, fmt.Errorf("parse sing-box pid file: %w", err)
		}
		return pid, nil
	}

	cmd := exec.Command("pgrep", "-o", "sing-box")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("resolve sing-box pid with pgrep: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("parse sing-box pid from pgrep: %w", err)
	}
	return pid, nil
}

type ClashMonitor struct {
	store      *database.Store
	cfg        config.MonitoringConfig
	limit      int
	notifier   AdminNotifier
	httpClient *http.Client
	logger     *zap.Logger
	notifiedAt map[int64]time.Time
}

func NewClashMonitor(store *database.Store, cfg config.MonitoringConfig, limit int, notifier AdminNotifier, logger *zap.Logger) *ClashMonitor {
	return &ClashMonitor{
		store:      store,
		cfg:        cfg,
		limit:      limit,
		notifier:   notifier,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
		notifiedAt: make(map[int64]time.Time),
	}
}

func (m *ClashMonitor) Run(ctx context.Context) {
	if !m.cfg.Enabled {
		m.logger.Info("clash monitor disabled")
		return
	}

	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	m.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *ClashMonitor) poll(ctx context.Context) {
	users, err := m.store.ListConfigUsers(ctx)
	if err != nil {
		m.logger.Error("load config users for monitor", zap.Error(err))
		return
	}

	resp, err := m.fetchConnections(ctx)
	if err != nil {
		m.logger.Error("fetch clash connections", zap.Error(err))
		return
	}

	usernameIndex := make(map[string]models.User, len(users)*2)
	for _, user := range users {
		usernameIndex[user.HY2Username] = user
		usernameIndex[user.SocksUsername] = user
	}

	counts := make(map[int64]int)
	lastIP := make(map[int64]string)
	lastSeen := make(map[int64]time.Time)
	extraConnections := make(map[int64][]string)

	for _, conn := range resp.Connections {
		username := extractConnectionUsername(conn)
		if username == "" {
			continue
		}
		user, ok := usernameIndex[username]
		if !ok {
			continue
		}
		counts[user.ID]++
		if conn.Metadata.DestinationIP != "" {
			lastIP[user.ID] = conn.Metadata.DestinationIP
		} else if conn.Metadata.SourceIP != "" {
			lastIP[user.ID] = conn.Metadata.SourceIP
		}
		lastSeen[user.ID] = time.Now().UTC()
		if counts[user.ID] > m.limit {
			extraConnections[user.ID] = append(extraConnections[user.ID], conn.ID)
		}
	}

	for _, user := range users {
		if count, ok := counts[user.ID]; ok {
			seen := lastSeen[user.ID]
			if err := m.store.UpdateConnectionState(ctx, user.ID, count, lastIP[user.ID], &seen); err != nil {
				m.logger.Warn("update connection state", zap.Int64("user_id", user.ID), zap.Error(err))
			}
		} else {
			if err := m.store.UpdateConnectionState(ctx, user.ID, 0, user.LastConnectedIP, user.LastConnectedAt); err != nil {
				m.logger.Warn("reset connection state", zap.Int64("user_id", user.ID), zap.Error(err))
			}
		}
	}

	for _, user := range users {
		extra := extraConnections[user.ID]
		if len(extra) == 0 {
			continue
		}
		m.handleLimitExceeded(ctx, user, counts[user.ID], extra)
	}
}

func (m *ClashMonitor) handleLimitExceeded(ctx context.Context, user models.User, count int, connectionIDs []string) {
	if m.cfg.DisconnectExtraSessions {
		sort.Strings(connectionIDs)
		for _, id := range connectionIDs {
			if err := m.closeConnection(ctx, id); err != nil {
				m.logger.Warn("disconnect extra connection", zap.String("connection_id", id), zap.Int64("user_id", user.ID), zap.Error(err))
			}
		}
	}

	if !m.cfg.AdminNotificationEnabled || m.notifier == nil {
		return
	}
	if last, ok := m.notifiedAt[user.ID]; ok && time.Since(last) < 15*time.Minute {
		return
	}
	m.notifiedAt[user.ID] = time.Now()

	text := fmt.Sprintf("User %d exceeded connection limit: %d active connections (limit=%d)", user.ID, count, m.limit)
	if err := m.notifier.NotifyAdmins(ctx, text); err != nil {
		m.logger.Warn("notify admins about connection limit", zap.Int64("user_id", user.ID), zap.Error(err))
	}
}

func (m *ClashMonitor) fetchConnections(ctx context.Context) (*clashConnectionsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.cfg.ClashAPIURL, "/")+"/connections", nil)
	if err != nil {
		return nil, fmt.Errorf("create clash request: %w", err)
	}
	res, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do clash request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("unexpected clash status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload clashConnectionsResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode clash response: %w", err)
	}
	return &payload, nil
}

func (m *ClashMonitor) closeConnection(ctx context.Context, connectionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(m.cfg.ClashAPIURL, "/")+"/connections/"+connectionID, nil)
	if err != nil {
		return fmt.Errorf("create delete connection request: %w", err)
	}
	res, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do delete connection request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("unexpected close connection status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type clashConnectionsResponse struct {
	Connections []clashConnection `json:"connections"`
}

type clashConnection struct {
	ID       string          `json:"id"`
	Metadata clashMetadata   `json:"metadata"`
	Raw      json.RawMessage `json:"-"`
}

type clashMetadata struct {
	User          string `json:"user"`
	SpecialProxy  string `json:"specialProxy"`
	Process       string `json:"process"`
	SourceIP      string `json:"sourceIP"`
	DestinationIP string `json:"destinationIP"`
	Host          string `json:"host"`
}

func extractConnectionUsername(connection clashConnection) string {
	if connection.Metadata.User != "" {
		return connection.Metadata.User
	}
	if connection.Metadata.SpecialProxy != "" {
		return connection.Metadata.SpecialProxy
	}
	return ""
}
