package lunixi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

// ErrInbox indicates a durable database failure. Return a retryable HTTP
// status, not a successful webhook acknowledgement, when it occurs.
var ErrInbox = errors.New("lunixi: webhook inbox persistence failed")

// ReplayStore applies a verified event and its event-ID marker in the SAME
// transaction. The callback must use the supplied transaction for business
// state. Network requests and other nontransactional effects belong in a
// durable application outbox, not inside this callback.
type ReplayStore interface {
	Process(context.Context, *WebhookEvent, func(*sql.Tx) error) (bool, error)
}

// SQLInbox is a durable ReplayStore for PostgreSQL and SQLite. Database
// drivers are supplied by the application; the SDK has no runtime dependency.
// Its table stores IDs, timestamp and a payload hash, never webhook raw data.
type SQLInbox struct {
	db           *sql.DB
	table, scope string
}

var sqlIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// NewSQLInbox validates a separate deduplication scope for each merchant or
// integration. table must be a trusted migration-time SQL identifier.
func NewSQLInbox(db *sql.DB, table, scope string) (*SQLInbox, error) {
	if db == nil || !sqlIdentifier.MatchString(table) || !safeHeader(scope, 200) {
		return nil, invalid("SQL inbox configuration")
	}
	return &SQLInbox{db: db, table: table, scope: scope}, nil
}

// EnsureSchema explicitly creates the inbox table. Production applications
// can apply the equivalent DDL in their normal migration process instead.
func (s *SQLInbox) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+s.table+" (scope TEXT NOT NULL, event_id TEXT NOT NULL, payload_sha256 TEXT NOT NULL, received_at TEXT NOT NULL, PRIMARY KEY (scope, event_id))")
	if err != nil {
		return ErrInbox
	}
	return nil
}

// Process returns true only when the callback and event marker are committed.
// A duplicate returns false without running callback. An event-ID collision
// with different signed content fails closed. Callback failure rolls back its
// marker so the provider can safely retry after an application crash/error.
func (s *SQLInbox) Process(ctx context.Context, event *WebhookEvent, apply func(*sql.Tx) error) (bool, error) {
	if event == nil || event.id == "" || len(event.raw) == 0 || apply == nil {
		return false, ErrWebhook
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, ErrInbox
	}
	defer tx.Rollback()
	hash := sha256.Sum256(event.raw)
	digest := hex.EncodeToString(hash[:])
	result, err := tx.ExecContext(ctx, "INSERT INTO "+s.table+" (scope,event_id,payload_sha256,received_at) VALUES ($1,$2,$3,$4) ON CONFLICT (scope,event_id) DO NOTHING", s.scope, event.id, digest, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, ErrInbox
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, ErrInbox
	}
	if affected == 0 {
		var existing string
		if err = tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM "+s.table+" WHERE scope=$1 AND event_id=$2", s.scope, event.id).Scan(&existing); err != nil {
			return false, ErrInbox
		}
		if existing != digest {
			return false, ErrWebhook
		}
		return false, nil
	}
	if err = apply(tx); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, ErrInbox
	}
	return true, nil
}
