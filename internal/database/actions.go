package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/service/credentials"
	"github.com/jackc/pgx/v5"
	"time"
)

type requestKey struct{}

func WithRequest(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestKey{}, id)
}
func (s *Store) Action(ctx context.Context, admin, userID int64, action string, days int, reason string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if id, ok := ctx.Value(requestKey{}).(string); ok {
		cmd, e := tx.Exec(ctx, "INSERT INTO admin_requests(request_id) VALUES($1) ON CONFLICT DO NOTHING", id)
		if e != nil {
			return e
		}
		if cmd.RowsAffected() == 0 {
			return tx.Commit(ctx)
		}
	}
	u, e := scanUser(tx.QueryRow(ctx, selectUserQuery+" WHERE id=$1 FOR UPDATE", userID))
	if e != nil {
		return e
	}
	changed, e := u.Transition(action, days, time.Now().UTC())
	if e != nil {
		return e
	}
	if !changed {
		return tx.Commit(ctx)
	}
	if action == "rotate" {
		u.HY2Password, e = credentials.Generate()
		if e != nil {
			return e
		}
		u.SocksPassword, e = credentials.Generate()
		if e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, "UPDATE users SET status=$2,expires_at=$3,hy2_password=$4,socks_password=$5,approved_at=CASE WHEN $6='approve' THEN NOW() ELSE approved_at END,approved_by=CASE WHEN $6='approve' THEN $7 ELSE approved_by END WHERE id=$1", u.ID, u.Status, u.ExpiresAt, u.HY2Password, u.SocksPassword, action, admin); e != nil {
		return e
	}
	// Audit stores action and bounded reason, never credentials or client links.
	if runes := []rune(reason); len(runes) > 256 {
		reason = string(runes[:256])
	}
	if _, e = tx.Exec(ctx, "INSERT INTO admin_logs(admin_id,target_user_id,action,reason) VALUES($1,$2,$3,$4)", admin, userID, action, reason); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) ApproveUser(ctx context.Context, a, u int64, d int) error {
	return s.Action(ctx, a, u, "approve", d, "")
}
func (s *Store) RejectUser(ctx context.Context, a, u int64, r string) error {
	return s.Action(ctx, a, u, "reject", 0, r)
}
func (s *Store) ExtendUser(ctx context.Context, a, u int64, d int, r string) error {
	return s.Action(ctx, a, u, "extend", d, r)
}
func (s *Store) SuspendUser(ctx context.Context, a, u int64) error {
	return s.Action(ctx, a, u, "suspend", 0, "")
}
func (s *Store) ActivateUser(ctx context.Context, a, u int64) error {
	return s.Action(ctx, a, u, "activate", 0, "")
}
func (s *Store) SelectNode(ctx context.Context, tg int64, node string) error {
	cmd, e := s.pool.Exec(ctx, "UPDATE users SET node_id=$2 WHERE telegram_id=$1 AND node_id<>$2", tg, node)
	if e != nil {
		return e
	}
	if cmd.RowsAffected() == 0 {
		_, e = s.GetUserByTelegramID(ctx, tg)
	}
	return e
}
func (s *Store) Search(ctx context.Context, q string) ([]models.User, error) {
	rows, e := s.pool.Query(ctx, selectUserQuery+" WHERE id::text=$1 OR telegram_id::text=$1 OR lower(telegram_username)=lower($2) ORDER BY id LIMIT 10", q, trimUsername(q))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return collectUsers(rows)
}
func trimUsername(q string) string {
	if len(q) > 0 && q[0] == '@' {
		return q[1:]
	}
	return q
}

type SyncState struct {
	NodeID, DesiredHash, AppliedHash string
	Revision                         int64
	LastSuccess                      *time.Time
	Healthy                          bool
	Error                            string
}

func (s *Store) SyncStates(ctx context.Context) ([]SyncState, error) {
	rows, e := s.pool.Query(ctx, "SELECT node_id,desired_hash,applied_hash,revision,last_success,(healthy AND revision=(SELECT revision FROM desired_state WHERE id=1)),error FROM node_sync ORDER BY node_id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var r []SyncState
	for rows.Next() {
		var v SyncState
		if e = rows.Scan(&v.NodeID, &v.DesiredHash, &v.AppliedHash, &v.Revision, &v.LastSuccess, &v.Healthy, &v.Error); e != nil {
			return nil, e
		}
		r = append(r, v)
	}
	return r, rows.Err()
}

// LockedSnapshot serializes reconcilers before reading the current desired state.
func (s *Store) LockedSnapshot(ctx context.Context, apply func([]models.User, int64, func(SyncState) error) error) error {
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(731092)"); e != nil {
		return e
	}
	// One SQL statement reads both revision and users from one MVCC snapshot.
	var revision int64
	var raw []byte
	if e = tx.QueryRow(ctx, `SELECT revision,COALESCE((SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM users u),'[]'::jsonb) FROM desired_state WHERE id=1`).Scan(&revision, &raw); e != nil {
		return e
	}
	// JSON tags mirror SQL for this snapshot; explicit mapping preserves nullable historical fields.
	var records []map[string]json.RawMessage
	if json.Unmarshal(raw, &records) != nil {
		return errors.New("invalid desired snapshot")
	}
	var users []models.User
	for _, r := range records {
		var u models.User
		for key, target := range map[string]any{"id": &u.ID, "status": &u.Status, "expires_at": &u.ExpiresAt, "hy2_password": &u.HY2Password, "socks_username": &u.SocksUsername, "socks_password": &u.SocksPassword, "node_id": &u.NodeID} {
			if e = json.Unmarshal(r[key], target); e != nil {
				return errors.New("invalid desired user")
			}
		}
		users = append(users, u)
	}
	record := func(v SyncState) error {
		_, e := tx.Exec(ctx, `INSERT INTO node_sync(node_id,desired_hash,applied_hash,revision,last_success,healthy,error) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(node_id) DO UPDATE SET desired_hash=EXCLUDED.desired_hash,applied_hash=CASE WHEN EXCLUDED.applied_hash='' THEN node_sync.applied_hash ELSE EXCLUDED.applied_hash END,revision=EXCLUDED.revision,last_success=COALESCE(EXCLUDED.last_success,node_sync.last_success),healthy=EXCLUDED.healthy,error=EXCLUDED.error`, v.NodeID, v.DesiredHash, v.AppliedHash, v.Revision, v.LastSuccess, v.Healthy, v.Error)
		return e
	}
	applyErr := apply(users, revision, record)
	if e = tx.Commit(ctx); e != nil {
		return fmt.Errorf("persist node sync: %w", e)
	}
	return applyErr
}
