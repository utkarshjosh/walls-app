package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// People who sign in to the app, and what they did there.
//
// Signing in needs no password: the server emails a short code and the person
// types it back. The first time an address does that, it becomes a user.

// ErrFull is returned when no more users can be added.
var ErrFull = errors.New("records: full")

// User statuses.
const (
	UserActive  = "active"
	UserBlocked = "blocked" // cannot sign in; existing sign-ins are ended
)

// MaxUsers stops the table growing without limit if sign-in is abused.
const MaxUsers = 1_000_000

// maxSessionsPerUser is how many signed-in devices one person keeps; signing
// in on another ends the oldest.
const maxSessionsPerUser = 10

// What a signed-in app may report. Anything else is dropped on arrival.
const (
	EventAppOpen      = "app_open"      // the app was opened; subject is its version, detail the system
	EventDiscoverOpen = "discover_open" // the Discover section was opened
	EventSearch       = "search"        // a search in Discover; detail is the text typed
	EventView         = "view"          // a wallpaper was opened full size in Discover; subject is its id
	EventSave         = "save"          // a wallpaper was saved to the library; subject is its id
	EventSet          = "set"           // a wallpaper was put on the desktop; subject is its id
	EventRemove       = "remove"        // a saved wallpaper was removed from the library; subject is its id
	EventPackOpen     = "pack_open"     // a collection was opened in Discover; subject is its id
	EventPackSave     = "pack_save"     // a whole collection was saved; subject is its id
)

// EventKinds is the set of kinds accepted from the app.
var EventKinds = map[string]bool{
	EventAppOpen: true, EventDiscoverOpen: true, EventSearch: true, EventView: true, EventSave: true,
	EventSet: true, EventRemove: true, EventPackOpen: true, EventPackSave: true,
}

// EventNeedsSubject is the kinds that mean nothing without the id of the
// wallpaper or collection they happened to.
var EventNeedsSubject = map[string]bool{
	EventView: true, EventSave: true, EventSet: true, EventRemove: true, EventPackOpen: true, EventPackSave: true,
}

// User is one person who has signed in.
type User struct {
	ID         int64
	Email      string
	Country    string // two letters when known, else ""
	Status     string
	Note       string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// UserRow is a user as the admin list shows them.
type UserRow struct {
	User
	Events int64 // how many things they have done, all time
}

// UserQuery filters the list of users.
type UserQuery struct {
	Search string // case-insensitive substring of the address or the note
	Status string // "", "active" or "blocked"
	Limit  int    // 0 means no limit
}

// UserTotals summarises the users.
type UserTotals struct {
	Total   int
	Blocked int
}

// UserEvent is one thing a signed-in person did in the app.
type UserEvent struct {
	At      time.Time
	Kind    string
	Subject string
	Detail  string
}

// SubjectReach is how often something happened to one item, and to how many
// different people.
type SubjectReach struct {
	Subject string
	N       int64
	People  int64
}

// --- sign-in codes ------------------------------------------------------------

// SaveLoginCode stores the one live code for an address, replacing any earlier
// one. Only a hash is kept, so reading this file does not reveal a code.
func (db *DB) SaveLoginCode(ctx context.Context, email, codeHash string, now, expires time.Time) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM login_codes WHERE expires_at <= ?`, stamp(now)); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `INSERT INTO login_codes (email, code_hash, attempts, expires_at) VALUES (?, ?, 0, ?)
		ON CONFLICT(email) DO UPDATE SET code_hash = excluded.code_hash, attempts = 0, expires_at = excluded.expires_at`,
		email, codeHash, stamp(expires))
	return err
}

// UseLoginCode checks a code someone typed. It reports true exactly once: for
// the right code, before it runs out, within maxAttempts tries. The code is
// then gone. Every try is counted first, so guessing cannot go on for longer
// than the allowance whatever order requests arrive in.
func (db *DB) UseLoginCode(ctx context.Context, email, codeHash string, now time.Time, maxAttempts int) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `UPDATE login_codes SET attempts = attempts + 1
		WHERE email = ? AND expires_at > ? AND attempts < ?`, email, stamp(now), maxAttempts)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil // no code, run out, or used up
	}
	res, err = db.sql.ExecContext(ctx, `DELETE FROM login_codes WHERE email = ? AND code_hash = ?`, email, codeHash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// --- users --------------------------------------------------------------------

const userCols = `id, email, country, status, note, created_at, last_seen_at`

func scanUser(row interface{ Scan(...any) error }, extra ...any) (User, error) {
	var u User
	var created, seen string
	if err := row.Scan(append([]any{&u.ID, &u.Email, &u.Country, &u.Status, &u.Note, &created, &seen}, extra...)...); err != nil {
		return u, err
	}
	u.CreatedAt, u.LastSeenAt = parseStamp(created), parseStamp(seen)
	return u, nil
}

func (db *DB) userWhere(ctx context.Context, where string, arg any) (User, error) {
	u, err := scanUser(db.sql.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// UserByID returns one user, or ErrNotFound.
func (db *DB) UserByID(ctx context.Context, id int64) (User, error) {
	return db.userWhere(ctx, `id = ?`, id)
}

// UserByEmail returns the user with this (already normalised) address, or ErrNotFound.
func (db *DB) UserByEmail(ctx context.Context, email string) (User, error) {
	return db.userWhere(ctx, `email = ?`, email)
}

// SignInUser returns the user for an address that has just proved it receives
// mail there, creating them the first time. It reports whether they are new.
func (db *DB) SignInUser(ctx context.Context, email, country string, now time.Time) (User, bool, error) {
	u, err := db.UserByEmail(ctx, email)
	if err == nil {
		if _, err := db.sql.ExecContext(ctx, `UPDATE users SET last_seen_at = ? WHERE id = ?`, stamp(now), u.ID); err != nil {
			return u, false, err
		}
		u.LastSeenAt = now.UTC().Truncate(time.Second)
		return u, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return u, false, err
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return u, false, err
	}
	if n >= MaxUsers {
		return u, false, ErrFull
	}
	res, err := db.sql.ExecContext(ctx, `INSERT INTO users (email, country, created_at, last_seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO NOTHING`, email, country, stamp(now), stamp(now))
	if err != nil {
		return u, false, fmt.Errorf("records: add user: %w", err)
	}
	created, _ := res.RowsAffected() // 0 when the same address signed in twice at once
	u, err = db.UserByEmail(ctx, email)
	return u, created > 0, err
}

// ListUsers returns users, newest first, each with how much they have done.
func (db *DB) ListUsers(ctx context.Context, q UserQuery) ([]UserRow, error) {
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
	query := `SELECT ` + userCols + `, (SELECT COUNT(*) FROM user_events e WHERE e.user_id = users.id)
		FROM users WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var r UserRow
		if r.User, err = scanUser(rows, &r.Events); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UserTotals counts all users and how many are blocked.
func (db *DB) UserTotals(ctx context.Context) (UserTotals, error) {
	var t UserTotals
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(status = 'blocked'), 0) FROM users`).Scan(&t.Total, &t.Blocked)
	return t, err
}

// UsersByDay counts new users per UTC day from fromDay onwards.
func (db *DB) UsersByDay(ctx context.Context, fromDay string) (map[string]int64, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT substr(created_at, 1, 10) AS day, COUNT(*) FROM users
		WHERE created_at >= ? GROUP BY day`, fromDay)
	if err != nil {
		return nil, err
	}
	return dayMap(rows)
}

// ActiveUsersByDay counts, per UTC day from fromDay onwards, how many
// different people did anything in the app.
func (db *DB) ActiveUsersByDay(ctx context.Context, fromDay string) (map[string]int64, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT substr(at, 1, 10) AS day, COUNT(DISTINCT user_id) FROM user_events
		WHERE at >= ? GROUP BY day`, fromDay)
	if err != nil {
		return nil, err
	}
	return dayMap(rows)
}

// ActiveUsersSince counts how many different people did anything from fromDay onwards.
func (db *DB) ActiveUsersSince(ctx context.Context, fromDay string) (int64, error) {
	var n int64
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM user_events WHERE at >= ?`, fromDay).Scan(&n)
	return n, err
}

// SetUserStatus blocks or unblocks one user. Blocking also ends every
// sign-in they have, so the app is signed out at its next request.
func (db *DB) SetUserStatus(ctx context.Context, id int64, status string) error {
	if status != UserActive && status != UserBlocked {
		return fmt.Errorf("records: unknown status %q", status)
	}
	res, err := db.sql.ExecContext(ctx, `UPDATE users SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if status == UserBlocked {
		_, err = db.sql.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ?`, id)
	}
	return err
}

// SetUserNote replaces the admin's note on one user.
func (db *DB) SetUserNote(ctx context.Context, id int64, note string) error {
	res, err := db.sql.ExecContext(ctx, `UPDATE users SET note = ? WHERE id = ?`, note, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes one user for good, with their sign-ins, any code waiting
// for them and everything recorded about what they did.
func (db *DB) DeleteUser(ctx context.Context, id int64) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A write first, so the transaction holds the write lock from the start.
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_codes WHERE email = (SELECT email FROM users WHERE id = ?)`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id) // sessions and events go with it
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// --- user sign-ins ------------------------------------------------------------

// CreateUserSession remembers a signed-in copy of the app by the hash of its
// token. The token itself is never stored.
func (db *DB) CreateUserSession(ctx context.Context, tokenHash string, userID int64, client string, now, expires time.Time) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM user_sessions WHERE expires_at <= ?`, stamp(now)); err != nil {
		return err
	}
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO user_sessions (token_hash, user_id, client, created_at, last_used_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, tokenHash, userID, client, stamp(now), stamp(now), stamp(expires)); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ? AND token_hash NOT IN (
		SELECT token_hash FROM user_sessions WHERE user_id = ? ORDER BY created_at DESC, token_hash LIMIT ?)`,
		userID, userID, maxSessionsPerUser)
	return err
}

// UserBySession returns the user a token belongs to and when the token was
// last used, or ErrNotFound when it is unknown or has run out.
func (db *DB) UserBySession(ctx context.Context, tokenHash string, now time.Time) (User, time.Time, error) {
	var used string
	u, err := scanUser(db.sql.QueryRowContext(ctx, `SELECT u.id, u.email, u.country, u.status, u.note, u.created_at, u.last_seen_at, s.last_used_at
		FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, stamp(now)), &used)
	if errors.Is(err, sql.ErrNoRows) {
		return u, time.Time{}, ErrNotFound
	}
	return u, parseStamp(used), err
}

// TouchUserSession notes that a sign-in was used and moves its end date on.
func (db *DB) TouchUserSession(ctx context.Context, tokenHash string, userID int64, now, expires time.Time) error {
	if _, err := db.sql.ExecContext(ctx, `UPDATE user_sessions SET last_used_at = ?, expires_at = ? WHERE token_hash = ?`,
		stamp(now), stamp(expires), tokenHash); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `UPDATE users SET last_seen_at = ? WHERE id = ?`, stamp(now), userID)
	return err
}

// DeleteUserSession signs one copy of the app out.
func (db *DB) DeleteUserSession(ctx context.Context, tokenHash string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// --- what users did -----------------------------------------------------------

// AddUserEvents records things one person did in the app.
func (db *DB) AddUserEvents(ctx context.Context, userID int64, events []UserEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO user_events (user_id, at, kind, subject, detail) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range events {
		if _, err := stmt.ExecContext(ctx, userID, stamp(e.At), e.Kind, e.Subject, e.Detail); err != nil {
			return fmt.Errorf("records: add event: %w", err)
		}
	}
	return tx.Commit()
}

// UserEvents returns the newest things one person did, newest first.
func (db *DB) UserEvents(ctx context.Context, userID int64, limit int) ([]UserEvent, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT at, kind, subject, detail FROM user_events
		WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserEvent{}
	for rows.Next() {
		var e UserEvent
		var at string
		if err := rows.Scan(&at, &e.Kind, &e.Subject, &e.Detail); err != nil {
			return nil, err
		}
		e.At = parseStamp(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// UserEventCounts counts what one person did, by kind.
func (db *DB) UserEventCounts(ctx context.Context, userID int64) (map[string]int64, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT kind, COUNT(*) FROM user_events WHERE user_id = ? GROUP BY kind`, userID)
	if err != nil {
		return nil, err
	}
	return dayMap(rows) // the same shape: a name and a number
}

// EventReach counts, for one kind of event from fromDay onwards ("" for all
// time), how often it happened to each subject and to how many different
// people. The subject most people did it to comes first.
func (db *DB) EventReach(ctx context.Context, kind, fromDay string) ([]SubjectReach, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT subject, COUNT(*), COUNT(DISTINCT user_id) AS people FROM user_events
		WHERE kind = ? AND at >= ? AND subject <> '' GROUP BY subject ORDER BY people DESC, COUNT(*) DESC, subject`, kind, fromDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubjectReach{}
	for rows.Next() {
		var c SubjectReach
		if err := rows.Scan(&c.Subject, &c.N, &c.People); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
