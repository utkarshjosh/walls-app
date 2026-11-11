// Package records is the second SQLite file, records.db: the list of people
// who asked for the build, daily counts of what the site and the app asked
// the server for, admin sign-ins, and a log of what the admin changed.
//
// It is kept apart from the catalog on purpose. catalog.db is rebuilt from the
// repository and replaced on every deploy; records.db exists only where the
// service runs and must never be overwritten.
package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// ErrNotFound is returned when a sign-up id does not exist.
var ErrNotFound = errors.New("records: not found")

// Sign-up statuses.
const (
	StatusNew     = "new"     // on the list, not sent the build yet
	StatusInvited = "invited" // the build has been sent
)

// Where a sign-up came from.
const (
	SourceSite   = "site"   // the form on the landing page
	SourceManual = "manual" // typed in by the admin
)

// Kinds of daily count.
const (
	CountVisit    = "visit"    // the landing page was opened
	CountDiscover = "discover" // the app asked for the list of collections
	CountFile     = "file"     // a wallpaper's full file was fetched; subject is the item id
	CountPack     = "pack"     // one collection was opened in full; subject is the pack id
)

// MaxSignups stops the list growing without limit if the form is abused.
const MaxSignups = 200_000

// dayLayout is how a day is written in the counts table, always in UTC.
const dayLayout = "2006-01-02"

// Day formats t as a UTC day.
func Day(t time.Time) string { return t.UTC().Format(dayLayout) }

// Signup is one person on the list.
type Signup struct {
	ID        int64
	Email     string
	Source    string
	Country   string // two letters when known, else ""
	Status    string
	Note      string
	CreatedAt time.Time
	InvitedAt time.Time // zero until the build has been sent
}

// SignupQuery filters the list.
type SignupQuery struct {
	Search string // case-insensitive substring of the address or the note
	Status string // "", "new" or "invited"
	Limit  int    // 0 means no limit
}

// SignupTotals summarises the list.
type SignupTotals struct {
	Total   int
	Invited int
}

// Event is one line of the admin log.
type Event struct {
	At     time.Time
	Action string
	Detail string
}

// SubjectCount is a count for one item or pack.
type SubjectCount struct {
	Subject string
	N       int64
}

// DB wraps the SQLite handle.
type DB struct {
	sql *sql.DB
}

// Open opens (and if needed creates and migrates) the records file at path.
func Open(path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("records: create folder: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	// FULL, not NORMAL as the catalog uses: a sign-up exists nowhere else, so
	// it must survive the machine losing power right after the form is sent.
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()

	sqldb, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("records: open: %w", err)
	}
	sqldb.SetMaxOpenConns(4)
	db := &DB{sql: sqldb}
	if err := db.migrate(context.Background()); err != nil {
		sqldb.Close()
		return nil, err
	}
	// The file holds people's addresses: keep it to the owner on a shared machine.
	_ = os.Chmod(abs, 0o600)
	return db, nil
}

// Close releases the database.
func (db *DB) Close() error { return db.sql.Close() }

// migrations run once each, in order. Append; never edit one that has shipped.
var migrations = []string{
	`CREATE TABLE signups (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		email      TEXT NOT NULL UNIQUE,
		source     TEXT NOT NULL,
		country    TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','invited')),
		note       TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		invited_at TEXT
	);
	CREATE INDEX signups_created ON signups (created_at);
	CREATE TABLE daily_counts (
		day     TEXT NOT NULL,
		kind    TEXT NOT NULL,
		subject TEXT NOT NULL DEFAULT '',
		n       INTEGER NOT NULL,
		PRIMARY KEY (day, kind, subject)
	) WITHOUT ROWID;
	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	) WITHOUT ROWID;
	CREATE TABLE events (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		at     TEXT NOT NULL,
		action TEXT NOT NULL,
		detail TEXT NOT NULL
	);`,
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("records: migrate: %w", err)
	}
	var have int
	if err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&have); err != nil {
		return fmt.Errorf("records: migrate: %w", err)
	}
	for v := have + 1; v <= len(migrations); v++ {
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v-1]); err != nil {
			tx.Rollback()
			return fmt.Errorf("records: migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			v, stamp(time.Now())); err != nil {
			tx.Rollback()
			return fmt.Errorf("records: migration %d: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func stamp(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }

func parseStamp(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// escapeLike makes user text safe inside a LIKE pattern (ESCAPE '\').
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// --- the sign-up list ---------------------------------------------------------

// AddSignup puts an address on the list. The address must already be
// normalised (see NormalizeEmail). It reports false, with no error, when the
// address was already there or the list is full: the caller answers the
// visitor the same way either way.
func (db *DB) AddSignup(ctx context.Context, email, source, country, note string, now time.Time) (bool, error) {
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups`).Scan(&n); err != nil {
		return false, err
	}
	if n >= MaxSignups {
		return false, nil
	}
	res, err := db.sql.ExecContext(ctx, `INSERT INTO signups (email, source, country, note, created_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(email) DO NOTHING`, email, source, country, note, stamp(now))
	if err != nil {
		return false, fmt.Errorf("records: add sign-up: %w", err)
	}
	added, _ := res.RowsAffected()
	return added > 0, nil
}

const signupCols = `id, email, source, country, status, note, created_at, invited_at`

func scanSignup(rows *sql.Rows) (Signup, error) {
	var s Signup
	var created string
	var invited sql.NullString
	if err := rows.Scan(&s.ID, &s.Email, &s.Source, &s.Country, &s.Status, &s.Note, &created, &invited); err != nil {
		return s, err
	}
	s.CreatedAt = parseStamp(created)
	if invited.Valid {
		s.InvitedAt = parseStamp(invited.String)
	}
	return s, nil
}

// ListSignups returns the list, newest first.
func (db *DB) ListSignups(ctx context.Context, q SignupQuery) ([]Signup, error) {
	where := []string{"1=1"}
	var args []any
	if s := strings.ToLower(strings.TrimSpace(q.Search)); s != "" {
		pat := "%" + escapeLike(s) + "%"
		where = append(where, `(email LIKE ? ESCAPE '\' OR lower(note) LIKE ? ESCAPE '\')`)
		args = append(args, pat, pat)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	query := `SELECT ` + signupCols + ` FROM signups WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Signup{}
	for rows.Next() {
		s, err := scanSignup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SignupTotals counts the whole list and how many have been sent the build.
func (db *DB) SignupTotals(ctx context.Context) (SignupTotals, error) {
	var t SignupTotals
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(status = 'invited'), 0) FROM signups`).Scan(&t.Total, &t.Invited)
	return t, err
}

// SignupsByDay counts sign-ups per UTC day from fromDay onwards.
func (db *DB) SignupsByDay(ctx context.Context, fromDay string) (map[string]int64, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT substr(created_at, 1, 10) AS day, COUNT(*) FROM signups
		WHERE created_at >= ? GROUP BY day`, fromDay)
	if err != nil {
		return nil, err
	}
	return dayMap(rows)
}

// SetStatus marks the given people as sent the build, or not. It returns how
// many rows changed.
func (db *DB) SetStatus(ctx context.Context, ids []int64, status string, now time.Time) (int, error) {
	if status != StatusNew && status != StatusInvited {
		return 0, fmt.Errorf("records: unknown status %q", status)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var invitedAt any
	if status == StatusInvited {
		invitedAt = stamp(now)
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	changed := 0
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE signups SET status = ?, invited_at = ? WHERE id = ? AND status <> ?`,
			status, invitedAt, id, status)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		changed += int(n)
	}
	return changed, tx.Commit()
}

// SetNote replaces the admin's note on one person.
func (db *DB) SetNote(ctx context.Context, id int64, note string) error {
	res, err := db.sql.ExecContext(ctx, `UPDATE signups SET note = ? WHERE id = ?`, note, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSignup removes one person from the list for good.
func (db *DB) DeleteSignup(ctx context.Context, id int64) error {
	res, err := db.sql.ExecContext(ctx, `DELETE FROM signups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- daily counts -------------------------------------------------------------

// Count adds one to today's count of kind for subject ("" when the kind has
// no subject). Nothing about who asked is stored: only the running total.
func (db *DB) Count(ctx context.Context, now time.Time, kind, subject string) error {
	_, err := db.sql.ExecContext(ctx, `INSERT INTO daily_counts (day, kind, subject, n) VALUES (?, ?, ?, 1)
		ON CONFLICT(day, kind, subject) DO UPDATE SET n = n + 1`, Day(now), kind, subject)
	return err
}

func dayMap(rows *sql.Rows) (map[string]int64, error) {
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var day string
		var n int64
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		out[day] = n
	}
	return out, rows.Err()
}

// CountsByDay sums a kind over all its subjects, per day, from fromDay onwards.
func (db *DB) CountsByDay(ctx context.Context, kind, fromDay string) (map[string]int64, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT day, SUM(n) FROM daily_counts
		WHERE kind = ? AND day >= ? GROUP BY day`, kind, fromDay)
	if err != nil {
		return nil, err
	}
	return dayMap(rows)
}

// CountsBySubject sums a kind per subject from fromDay onwards ("" for all
// time), largest first.
func (db *DB) CountsBySubject(ctx context.Context, kind, fromDay string) ([]SubjectCount, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT subject, SUM(n) AS total FROM daily_counts
		WHERE kind = ? AND day >= ? GROUP BY subject ORDER BY total DESC, subject`, kind, fromDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubjectCount{}
	for rows.Next() {
		var c SubjectCount
		if err := rows.Scan(&c.Subject, &c.N); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- admin sessions -----------------------------------------------------------

// CreateSession remembers a signed-in browser by the hash of its token. The
// token itself is never stored, so reading this file does not let anyone in.
func (db *DB) CreateSession(ctx context.Context, tokenHash string, now, expires time.Time) error {
	// Old sessions are cleared whenever a new one starts; nothing else needs to.
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, stamp(now)); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `INSERT INTO sessions (token_hash, created_at, expires_at) VALUES (?, ?, ?)`,
		tokenHash, stamp(now), stamp(expires))
	return err
}

// SessionValid reports whether a session with this hash exists and has not run out.
func (db *DB) SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var one int
	err := db.sql.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, stamp(now)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// DeleteSession signs one browser out.
func (db *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// --- the admin log ------------------------------------------------------------

// maxEvents is how many log lines are kept; older ones are dropped.
const maxEvents = 2000

// Log records something the admin did. Details name counts, never addresses,
// so removing a person really removes them.
func (db *DB) Log(ctx context.Context, now time.Time, action, detail string) error {
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO events (at, action, detail) VALUES (?, ?, ?)`,
		stamp(now), action, detail); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `DELETE FROM events WHERE id <= (SELECT MAX(id) FROM events) - ?`, maxEvents)
	return err
}

// RecentEvents returns the newest log lines first.
func (db *DB) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT at, action, detail FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var at string
		if err := rows.Scan(&at, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		e.At = parseStamp(at)
		out = append(out, e)
	}
	return out, rows.Err()
}
