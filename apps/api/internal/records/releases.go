package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Builds of the app offered for download.
//
// A release is one version of the app. It holds one build per platform (the
// file people download). A release is a draft until it is published; the
// newest published release is the one the website offers.

// ErrExists is returned when something with the same name is already there.
var ErrExists = errors.New("records: already exists")

// Release channels: what the website calls the release.
const (
	ChannelBeta   = "beta"
	ChannelStable = "stable"
)

// Release is one version of the app.
type Release struct {
	ID          int64
	Version     string
	Channel     string
	Notes       string
	CreatedAt   time.Time
	PublishedAt time.Time // zero while it is a draft
	Builds      []Build
}

// Published reports whether the release is offered for download.
func (r Release) Published() bool { return !r.PublishedAt.IsZero() }

// Build is one downloadable file of a release.
type Build struct {
	ID        int64
	ReleaseID int64
	Platform  string // e.g. "mac-arm64"
	Filename  string
	Bytes     int64
	SHA256    string
	CreatedAt time.Time
}

const releaseCols = `id, version, channel, notes, created_at, published_at`

func scanRelease(row interface{ Scan(...any) error }) (Release, error) {
	var r Release
	var created string
	var published sql.NullString
	if err := row.Scan(&r.ID, &r.Version, &r.Channel, &r.Notes, &created, &published); err != nil {
		return r, err
	}
	r.CreatedAt = parseStamp(created)
	if published.Valid {
		r.PublishedAt = parseStamp(published.String)
	}
	r.Builds = []Build{}
	return r, nil
}

const buildCols = `id, release_id, platform, filename, bytes, sha256, created_at`

func scanBuild(row interface{ Scan(...any) error }) (Build, error) {
	var b Build
	var created string
	if err := row.Scan(&b.ID, &b.ReleaseID, &b.Platform, &b.Filename, &b.Bytes, &b.SHA256, &created); err != nil {
		return b, err
	}
	b.CreatedAt = parseStamp(created)
	return b, nil
}

func (db *DB) buildsOf(ctx context.Context, releaseID int64) ([]Build, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+buildCols+` FROM builds WHERE release_id = ? ORDER BY platform`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateRelease starts a draft release. ErrExists means the version is taken.
func (db *DB) CreateRelease(ctx context.Context, version, channel, notes string, now time.Time) (Release, error) {
	res, err := db.sql.ExecContext(ctx, `INSERT INTO releases (version, channel, notes, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(version) DO NOTHING`, version, channel, notes, stamp(now))
	if err != nil {
		return Release{}, fmt.Errorf("records: add release: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Release{}, ErrExists
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Release{}, err
	}
	return db.Release(ctx, id)
}

// Release returns one release with its builds, or ErrNotFound.
func (db *DB) Release(ctx context.Context, id int64) (Release, error) {
	r, err := scanRelease(db.sql.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.Builds, err = db.buildsOf(ctx, r.ID)
	return r, err
}

// Releases returns every release with its builds, newest first.
func (db *DB) Releases(ctx context.Context) ([]Release, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+releaseCols+` FROM releases ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	out := []Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Builds, err = db.buildsOf(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// LatestRelease returns the release the website offers: the one published
// most recently. ErrNotFound means nothing is published.
func (db *DB) LatestRelease(ctx context.Context) (Release, error) {
	r, err := scanRelease(db.sql.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases
		WHERE published_at IS NOT NULL ORDER BY published_at DESC, id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.Builds, err = db.buildsOf(ctx, r.ID)
	return r, err
}

// UpdateRelease changes a release's channel and notes.
func (db *DB) UpdateRelease(ctx context.Context, id int64, channel, notes string) error {
	res, err := db.sql.ExecContext(ctx, `UPDATE releases SET channel = ?, notes = ? WHERE id = ?`, channel, notes, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetReleasePublished offers a release for download, or takes it back. A
// release that is published again moves to the front.
func (db *DB) SetReleasePublished(ctx context.Context, id int64, published bool, now time.Time) error {
	var at any
	if published {
		at = stamp(now)
	}
	res, err := db.sql.ExecContext(ctx, `UPDATE releases SET published_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRelease removes a release and returns the builds it had, so the
// caller can remove their files.
func (db *DB) DeleteRelease(ctx context.Context, id int64) ([]Build, error) {
	builds, err := db.buildsOf(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := db.sql.ExecContext(ctx, `DELETE FROM releases WHERE id = ?`, id) // its builds go with it
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return builds, nil
}

// PutBuild records a build of a release for one platform. If that platform
// already had a build, it is replaced and the old one is returned so the
// caller can remove its file.
func (db *DB) PutBuild(ctx context.Context, releaseID int64, platform, filename string, bytes int64, sha256 string, now time.Time) (Build, *Build, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Build{}, nil, err
	}
	defer tx.Rollback()
	// The delete comes first, so the transaction holds the write lock from the start.
	var replaced *Build
	old, err := scanBuild(tx.QueryRowContext(ctx, `DELETE FROM builds WHERE release_id = ? AND platform = ? RETURNING `+buildCols,
		releaseID, platform))
	switch {
	case err == nil:
		replaced = &old
	case !errors.Is(err, sql.ErrNoRows):
		return Build{}, nil, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO builds (release_id, platform, filename, bytes, sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, releaseID, platform, filename, bytes, sha256, stamp(now))
	if err != nil {
		return Build{}, nil, fmt.Errorf("records: add build: %w", err) // includes "no such release"
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Build{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return Build{}, nil, err
	}
	return Build{ID: id, ReleaseID: releaseID, Platform: platform, Filename: filename, Bytes: bytes, SHA256: sha256,
		CreatedAt: now.UTC().Truncate(time.Second)}, replaced, nil
}

// Build returns one build and the release it belongs to, or ErrNotFound.
func (db *DB) Build(ctx context.Context, id int64) (Build, Release, error) {
	b, err := scanBuild(db.sql.QueryRowContext(ctx, `SELECT `+buildCols+` FROM builds WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return b, Release{}, ErrNotFound
	}
	if err != nil {
		return b, Release{}, err
	}
	r, err := scanRelease(db.sql.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases WHERE id = ?`, b.ReleaseID))
	return b, r, err
}

// DeleteBuild removes one build and returns it, so the caller can remove its file.
func (db *DB) DeleteBuild(ctx context.Context, id int64) (Build, error) {
	b, err := scanBuild(db.sql.QueryRowContext(ctx, `DELETE FROM builds WHERE id = ? RETURNING `+buildCols, id))
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}
