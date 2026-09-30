package journal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go driver: the binary stays static
)

// SQLite is the single-node Store (RFC-0003, "Storage"). Every write is one
// statement or one transaction, so a crash leaves either the whole record
// or nothing.
type SQLite struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id           TEXT PRIMARY KEY,
	namespace    TEXT NOT NULL,
	function     TEXT NOT NULL,
	generation   TEXT NOT NULL,
	conversation TEXT NOT NULL,
	scopes       BLOB NOT NULL,
	status       TEXT NOT NULL,
	pending      TEXT NOT NULL DEFAULT '',
	acked        INTEGER NOT NULL DEFAULT 0,
	wake_at      TEXT,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_conversation ON runs (conversation, status);
CREATE TABLE IF NOT EXISTS records (
	run_id TEXT NOT NULL REFERENCES runs (id),
	seq    INTEGER NOT NULL,
	site   TEXT NOT NULL,
	kind   TEXT NOT NULL,
	value  BLOB,
	at     TEXT NOT NULL,
	PRIMARY KEY (run_id, seq),
	UNIQUE (run_id, site, kind)
);
CREATE TABLE IF NOT EXISTS milestones (
	namespace  TEXT NOT NULL,
	subject    TEXT NOT NULL,
	subject_id TEXT NOT NULL,
	name       TEXT NOT NULL,
	at         TEXT NOT NULL,
	attributes TEXT NOT NULL,
	run_id     TEXT NOT NULL,
	site       TEXT NOT NULL,
	UNIQUE (run_id, site)
);
CREATE INDEX IF NOT EXISTS milestones_by_subject ON milestones (namespace, subject, at);
CREATE TABLE IF NOT EXISTS watches (
	namespace TEXT NOT NULL,
	event     TEXT NOT NULL,
	key       TEXT NOT NULL,
	run_id    TEXT NOT NULL,
	site      TEXT NOT NULL,
	PRIMARY KEY (namespace, event, key, run_id, site)
);
CREATE INDEX IF NOT EXISTS watches_by_run ON watches (run_id);
CREATE TABLE IF NOT EXISTS cursors (name TEXT PRIMARY KEY, value INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS deliveries (conversation TEXT NOT NULL, message_id TEXT NOT NULL, PRIMARY KEY (conversation, message_id));`

// OpenSQLite opens (and creates) a database file. Use ":memory:" in tests.
func OpenSQLite(path string) (*SQLite, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One writer: SQLite serializes writes anyway, and one connection keeps
	// ":memory:" a single database.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	// Databases created before timers have no wake_at column.
	if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN wake_at TEXT`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	// Only sleeping runs are indexed: millions of finished runs cost nothing here.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS runs_wake ON runs (wake_at) WHERE wake_at IS NOT NULL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	return &SQLite{db: db}, nil
}

// Close closes the database.
func (s *SQLite) Close() error { return s.db.Close() }

// Times are stored in UTC with a fixed width, so text order is time order
// (RFC 3339 with trimmed fractions is not: "05Z" sorts after "05.1Z").
const (
	timeFormat = time.RFC3339Nano // parsing accepts the fixed-width form
	timeWrite  = "2006-01-02T15:04:05.000000000Z"
)

func (s *SQLite) CreateRun(ctx context.Context, r Run) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, namespace, function, generation, conversation, scopes, status, pending, acked, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Namespace, r.Function, r.Generation, r.Conversation, r.Scopes, string(r.Status), r.Pending, r.Acked,
		r.CreatedAt.UTC().Format(timeWrite), r.UpdatedAt.UTC().Format(timeWrite))
	if isUnique(err) {
		return fmt.Errorf("%w: %s", ErrExists, r.ID)
	}
	return err
}

func (s *SQLite) GetRun(ctx context.Context, id string) (Run, error) {
	var r Run
	var status, created, updated string
	var wake sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, namespace, function, generation, conversation, scopes, status, pending, acked, wake_at, created_at, updated_at
		 FROM runs WHERE id = ?`, id).
		Scan(&r.ID, &r.Namespace, &r.Function, &r.Generation, &r.Conversation, &r.Scopes, &status, &r.Pending, &r.Acked, &wake, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Run{}, err
	}
	r.Status = Status(status)
	if wake.Valid {
		if r.WakeAt, err = time.Parse(timeFormat, wake.String); err != nil {
			return Run{}, err
		}
	}
	if r.CreatedAt, err = time.Parse(timeFormat, created); err != nil {
		return Run{}, err
	}
	if r.UpdatedAt, err = time.Parse(timeFormat, updated); err != nil {
		return Run{}, err
	}
	return r, nil
}

func (s *SQLite) UpdateRun(ctx context.Context, id string, status Status, pending string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, pending = ?, wake_at = NULL, updated_at = ? WHERE id = ?`,
		string(status), pending, time.Now().UTC().Format(timeWrite), id)
	return notFound(res, err, id)
}

func (s *SQLite) Sleep(ctx context.Context, id string, status Status, pending string, wake time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, pending = ?, wake_at = ?, updated_at = ? WHERE id = ?`,
		string(status), pending, wake.UTC().Format(timeWrite), time.Now().UTC().Format(timeWrite), id)
	return notFound(res, err, id)
}

func (s *SQLite) DueRuns(ctx context.Context, now time.Time, generation string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM runs WHERE wake_at IS NOT NULL AND wake_at <= ? AND status IN (?, ?, ?, ?) AND (? = '' OR generation = ?)
		ORDER BY wake_at, rowid LIMIT ?`,
		now.UTC().Format(timeWrite), string(StatusWaitingForTime), string(StatusWaitingForUser), string(StatusWaitingForHuman), string(StatusIndeterminate),
		generation, generation, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(ids))
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *SQLite) Append(ctx context.Context, runID string, r Record) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE id = ?`, runID).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	var seq int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM records WHERE run_id = ?`, runID).Scan(&seq); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO records (run_id, seq, site, kind, value, at) VALUES (?, ?, ?, ?, ?, ?)`,
		runID, seq, r.Site, string(r.Kind), r.Value, r.At.UTC().Format(timeWrite))
	if isUnique(err) {
		return 0, fmt.Errorf("%w: %s %s", ErrDuplicate, r.Kind, r.Site)
	}
	if err != nil {
		return 0, err
	}
	if r.Kind == KindMilestone { // indexed in the same transaction as the record
		ms, err := MilestoneOf(runID, r)
		if err != nil {
			return 0, err
		}
		attrs, err := json.Marshal(ms.Attributes)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO milestones (namespace, subject, subject_id, name, at, attributes, run_id, site) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ms.Namespace, ms.Subject, ms.ID, ms.Name, ms.At.UTC().Format(timeWrite), string(attrs), runID, r.Site); err != nil {
			return 0, err
		}
	}
	return seq, tx.Commit()
}

func (s *SQLite) Runs(ctx context.Context, f RunFilter) ([]Run, error) {
	q := `SELECT id FROM runs WHERE 1 = 1`
	var args []any
	for _, c := range []struct{ col, v string }{{"namespace", f.Namespace}, {"conversation", f.Conversation}, {"function", f.Function}, {"status", string(f.Status)}} {
		if c.v != "" {
			q += " AND " + c.col + " = ?"
			args = append(args, c.v)
		}
	}
	if !f.Before.IsZero() {
		q += " AND created_at < ?"
		args = append(args, f.Before.UTC().Format(timeWrite))
	}
	q += " ORDER BY created_at DESC, rowid DESC LIMIT ?"
	args = append(args, f.limit())
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(ids))
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *SQLite) Unacked(ctx context.Context, prefix string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id FROM runs r WHERE substr(r.conversation, 1, length(?)) = ? AND EXISTS (
		SELECT 1 FROM records rec WHERE rec.run_id = r.id AND rec.seq > r.acked AND rec.kind IN (?, ?))
		ORDER BY r.rowid LIMIT ?`, prefix, prefix, string(KindSend), string(KindSuspend), limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(ids))
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *SQLite) Milestones(ctx context.Context, namespace, subject string) ([]Milestone, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subject_id, name, at, attributes, run_id, site FROM milestones
		WHERE namespace = ? AND subject = ? ORDER BY at, rowid`, namespace, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		ms := Milestone{Namespace: namespace, Subject: subject}
		var at, attrs string
		if err := rows.Scan(&ms.ID, &ms.Name, &at, &attrs, &ms.RunID, &ms.Site); err != nil {
			return nil, err
		}
		if ms.At, err = time.Parse(timeFormat, at); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(attrs), &ms.Attributes); err != nil {
			return nil, err
		}
		out = append(out, ms)
	}
	return out, rows.Err()
}

func (s *SQLite) Records(ctx context.Context, runID string) ([]Record, error) {
	if _, err := s.GetRun(ctx, runID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, site, kind, value, at FROM records WHERE run_id = ? ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var kind, at string
		if err := rows.Scan(&r.Seq, &r.Site, &kind, &r.Value, &at); err != nil {
			return nil, err
		}
		r.Kind = Kind(kind)
		if r.At, err = time.Parse(timeFormat, at); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) Ack(ctx context.Context, runID string, seq int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE runs SET acked = MAX(acked, ?) WHERE id = ?`, seq, runID)
	return notFound(res, err, runID)
}

func (s *SQLite) ActiveRun(ctx context.Context, conversation string) (Run, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM runs WHERE conversation = ? AND status NOT IN (?, ?, ?) ORDER BY created_at DESC, rowid DESC LIMIT 1`,
		conversation, string(StatusSucceeded), string(StatusFailed), string(StatusWaitingForTime)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	r, err := s.GetRun(ctx, id)
	return r, err == nil, err
}

func (s *SQLite) LastRun(ctx context.Context, conversation string) (Run, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM runs WHERE conversation = ? ORDER BY created_at DESC, rowid DESC LIMIT 1`, conversation).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	r, err := s.GetRun(ctx, id)
	return r, err == nil, err
}

func notFound(res sql.Result, err error, id string) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *SQLite) Watch(ctx context.Context, w Watch) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO watches (namespace, event, key, run_id, site) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		w.Namespace, w.Event, w.Key, w.RunID, w.Site)
	return err
}

func (s *SQLite) Watchers(ctx context.Context, namespace, event, key string) ([]Watch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, site FROM watches WHERE namespace = ? AND event = ? AND key = ? ORDER BY rowid`, namespace, event, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Watch
	for rows.Next() {
		w := Watch{Namespace: namespace, Event: event, Key: key}
		if err := rows.Scan(&w.RunID, &w.Site); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *SQLite) Unwatch(ctx context.Context, runID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE run_id = ?`, runID)
	return err
}

func (s *SQLite) MilestonesAfter(ctx context.Context, namespace string, after int64, limit int) ([]Milestone, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rowid, subject, subject_id, name, at, attributes, run_id, site FROM milestones
		WHERE namespace = ? AND rowid > ? ORDER BY rowid LIMIT ?`, namespace, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Milestone
	for rows.Next() {
		ms := Milestone{Namespace: namespace}
		var at, attrs string
		if err := rows.Scan(&ms.Seq, &ms.Subject, &ms.ID, &ms.Name, &at, &attrs, &ms.RunID, &ms.Site); err != nil {
			return nil, err
		}
		if ms.At, err = time.Parse(timeFormat, at); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(attrs), &ms.Attributes); err != nil {
			return nil, err
		}
		out = append(out, ms)
	}
	return out, rows.Err()
}

func (s *SQLite) LastMilestone(ctx context.Context, namespace string) (int64, error) {
	var last sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(rowid) FROM milestones WHERE namespace = ?`, namespace).Scan(&last)
	return last.Int64, err
}

func (s *SQLite) Cursor(ctx context.Context, name string) (int64, bool, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT value FROM cursors WHERE name = ?`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return v, err == nil, err
}

func (s *SQLite) SetCursor(ctx context.Context, name string, v int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO cursors (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value`, name, v)
	return err
}

func (s *SQLite) Changes(ctx context.Context, namespace string, after int64, limit int) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rec.rowid, rec.run_id, rec.seq, rec.site, rec.kind, rec.value, rec.at
		FROM records rec JOIN runs r ON r.id = rec.run_id
		WHERE r.namespace = ? AND rec.rowid > ? ORDER BY rec.rowid LIMIT ?`, namespace, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Change
	for rows.Next() {
		var c Change
		var kind, at string
		if err := rows.Scan(&c.Cursor, &c.RunID, &c.Record.Seq, &c.Record.Site, &kind, &c.Record.Value, &at); err != nil {
			return nil, err
		}
		c.Record.Kind = Kind(kind)
		if c.Record.At, err = time.Parse(timeFormat, at); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) FirstDelivery(ctx context.Context, conversation, messageID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO deliveries (conversation, message_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, conversation, messageID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
