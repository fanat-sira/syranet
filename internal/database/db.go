package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/models"
)

type Store struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

type TelegramProfile struct {
	TelegramID       int64
	TelegramUsername string
	FirstName        string
	LastName         string
}

func New(ctx context.Context, cfg config.DatabaseConfig, logger *zap.Logger) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig((&config.Config{Database: cfg}).DSN())
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxOpenConns
	poolCfg.MinConns = cfg.MinOpenConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{pool: pool, logger: logger}, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) UpsertTelegramUser(ctx context.Context, profile TelegramProfile, hy2Username, hy2Password, socksUsername, socksPassword string) (*models.User, error) {
	query := `
INSERT INTO users (
	telegram_id,
	telegram_username,
	first_name,
	last_name,
	hy2_username,
	hy2_password,
	socks_username,
	socks_password
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (telegram_id)
DO UPDATE SET
	telegram_username = EXCLUDED.telegram_username,
	first_name = EXCLUDED.first_name,
	last_name = EXCLUDED.last_name,
	updated_at = NOW()
RETURNING id, telegram_id, telegram_username, first_name, last_name, hy2_username, hy2_password,
	      socks_username, socks_password, status, created_at, updated_at, approved_at, approved_by,
	      expires_at, referral_source, intended_use, accepted_terms, last_connected_at, last_connected_ip,
	      current_connections, node_id`

	row := s.pool.QueryRow(ctx, query,
		profile.TelegramID,
		profile.TelegramUsername,
		profile.FirstName,
		profile.LastName,
		hy2Username,
		hy2Password,
		socksUsername,
		socksPassword,
	)

	user, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("upsert telegram user: %w", err)
	}
	return user, nil
}

func (s *Store) UpdateQuestionnaire(ctx context.Context, telegramID int64, referralSource, intendedUse string, acceptedTerms bool) error {
	cmd, err := s.pool.Exec(ctx, `
UPDATE users
SET referral_source = $2,
	intended_use = $3,
	accepted_terms = $4,
	status = $5,
	updated_at = NOW()
WHERE telegram_id = $1 AND status = 'pending' AND accepted_terms = false`, telegramID, referralSource, intendedUse, acceptedTerms, models.UserStatusPending)
	if err != nil {
		return fmt.Errorf("update questionnaire: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	row := s.pool.QueryRow(ctx, selectUserQuery+` WHERE telegram_id = $1`, telegramID)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	row := s.pool.QueryRow(ctx, selectUserQuery+` WHERE id = $1`, id)
	return scanUser(row)
}

func (s *Store) ListPendingUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, selectUserQuery+` WHERE status = $1 ORDER BY created_at ASC LIMIT 20`, models.UserStatusPending)
	if err != nil {
		return nil, fmt.Errorf("list pending users: %w", err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func (s *Store) ListUsersByStatuses(ctx context.Context, statuses []models.UserStatus, limit, offset int) ([]models.User, int, error) {
	statusValues := make([]string, 0, len(statuses))
	for _, status := range statuses {
		statusValues = append(statusValues, string(status))
	}

	rows, err := s.pool.Query(ctx, selectUserQuery+`
 WHERE status = ANY($1)
 ORDER BY created_at DESC
 LIMIT $2 OFFSET $3`, statusValues, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list users by statuses: %w", err)
	}
	defer rows.Close()

	users, err := collectUsers(rows)
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE status = ANY($1)`, statusValues).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users by statuses: %w", err)
	}

	return users, total, nil
}

func (s *Store) GetStats(ctx context.Context) (*models.UserStats, error) {
	stats := &models.UserStats{}
	query := `
SELECT
	COUNT(*) AS total,
	COUNT(*) FILTER (WHERE status = 'active') AS active,
	COUNT(*) FILTER (WHERE status = 'pending') AS pending,
	COUNT(*) FILTER (WHERE status = 'expired') AS expired,
	COUNT(*) FILTER (WHERE status = 'revoked') AS revoked,
	COUNT(*) FILTER (WHERE status = 'rejected') AS rejected,
	COUNT(*) FILTER (WHERE status = 'suspended') AS suspended
FROM users`

	if err := s.pool.QueryRow(ctx, query).Scan(
		&stats.Total,
		&stats.Active,
		&stats.Pending,
		&stats.Expired,
		&stats.Revoked,
		&stats.Rejected,
		&stats.Suspended,
	); err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return stats, nil
}

func (s *Store) ExpireUsers(ctx context.Context) (int64, error) {
	cmd, err := s.pool.Exec(ctx, `
UPDATE users
SET status = $1,
	updated_at = NOW()
WHERE status = $2
  AND expires_at IS NOT NULL
  AND expires_at < NOW()`, models.UserStatusExpired, models.UserStatusActive)
	if err != nil {
		return 0, fmt.Errorf("expire users: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func (s *Store) ListConfigUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, selectUserQuery+`
 WHERE status = $1
   AND expires_at IS NOT NULL
   AND expires_at > NOW()
 ORDER BY id ASC`, models.UserStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list config users: %w", err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func (s *Store) ListUsersExpiringBefore(ctx context.Context, before time.Time) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, selectUserQuery+`
 WHERE status = $1
   AND expires_at IS NOT NULL
   AND expires_at <= $2
 ORDER BY expires_at ASC`, models.UserStatusActive, before)
	if err != nil {
		return nil, fmt.Errorf("list expiring users: %w", err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func (s *Store) Health(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func collectUsers(rows pgx.Rows) ([]models.User, error) {
	users := make([]models.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (*models.User, error) {
	var user models.User
	if err := scanner.Scan(
		&user.ID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.FirstName,
		&user.LastName,
		&user.HY2Username,
		&user.HY2Password,
		&user.SocksUsername,
		&user.SocksPassword,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.ApprovedAt,
		&user.ApprovedBy,
		&user.ExpiresAt,
		&user.ReferralSource,
		&user.IntendedUse,
		&user.AcceptedTerms,
		&user.LastConnectedAt,
		&user.LastConnectedIP,
		&user.CurrentConnections,
		&user.NodeID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}

const selectUserQuery = `
SELECT id, telegram_id, telegram_username, first_name, last_name, hy2_username, hy2_password,
       socks_username, socks_password, status, created_at, updated_at, approved_at, approved_by,
       expires_at, referral_source, intended_use, accepted_terms, last_connected_at, last_connected_ip,
       current_connections, node_id
FROM users`
