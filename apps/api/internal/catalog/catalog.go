// Package catalog is the SQLite-backed index of wallpapers, categories and
// curated packs. It stores facts about items; the files themselves live in
// the object store.
package catalog

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// ErrNotFound is returned when an item or pack id does not exist.
var ErrNotFound = errors.New("catalog: not found")

// ErrBadCursor is returned when a paging cursor cannot be decoded.
var ErrBadCursor = errors.New("catalog: invalid cursor")

// Item kinds.
const (
	KindImage = "image"
	KindVideo = "video"
	KindScene = "scene"
)

// Item is one wallpaper in the catalog.
type Item struct {
	ID           string // first 12 hex chars of SHA-256 of the original bytes
	Title        string
	Kind         string // image | video | scene
	Width        int    // pixels of the original (poster for video/scene)
	Height       int
	Bytes        int64 // size of the original
	StoredBytes  int64 // original + every derivative, used for the storage cap
	Category     string
	Tags         []string
	Author       string
	License      string
	SourceURL    string // "" means none
	Color        string // "#rrggbb"
	Rank         int64  // feed order, ascending
	OriginalExt  string // "jpg", "mp4", "html", ...
	OriginalType string // content type of the original
	HasPoster    bool
	Origin       string // "seed", "scene" or "ingest": who put it here
	// Layout says how a live scene behaves when the wallpaper covers several
	// displays: "canvas" (one picture across them) or "per-display" (drawn
	// whole on each). Empty for images and videos.
	Layout    string
	CreatedAt time.Time
}

// Category is a slug with its display name and how many items use it.
type Category struct {
	Slug  string
	Name  string
	Count int
}

// Pack is a curated, ordered set of items.
type Pack struct {
	ID          string
	Name        string
	Description string
	Position    int
	Count       int
	CoverIDs    []string // first three item ids, in pack order
}

// Query filters and pages the item list.
type Query struct {
	Kind     string // "", "image", "live" (video or scene), "video", "scene"
	Category string
	Q        string // case-insensitive substring over title, tags, category, author
	Wide     bool   // width/height >= 2.6
	Cursor   string
	Limit    int
}

// Page is one page of results. NextCursor is "" on the last page.
type Page struct {
	Items      []Item
	NextCursor string
}

// DB wraps the SQLite handle.
type DB struct {
	sql *sql.DB
}

// Open opens (and if needed creates and migrates) the catalog at path.
func Open(path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("catalog: create folder: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()

	sqldb, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("catalog: open: %w", err)
	}
	sqldb.SetMaxOpenConns(4)
	db := &DB{sql: sqldb}
	if err := db.migrate(context.Background()); err != nil {
		sqldb.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the database.
func (db *DB) Close() error { return db.sql.Close() }

// migrations run once each, in order. Append; never edit one that has shipped.
var migrations = []string{
	`CREATE TABLE categories (
		slug TEXT PRIMARY KEY,
		name TEXT NOT NULL
	);
	CREATE TABLE items (
		id            TEXT PRIMARY KEY,
		title         TEXT NOT NULL,
		kind          TEXT NOT NULL CHECK (kind IN ('image','video','scene')),
		width         INTEGER NOT NULL CHECK (width > 0),
		height        INTEGER NOT NULL CHECK (height > 0),
		bytes         INTEGER NOT NULL,
		stored_bytes  INTEGER NOT NULL,
		category      TEXT NOT NULL,
		tags          TEXT NOT NULL,
		author        TEXT NOT NULL,
		license       TEXT NOT NULL,
		source_url    TEXT,
		color         TEXT NOT NULL,
		rank          INTEGER NOT NULL,
		original_ext  TEXT NOT NULL,
		original_type TEXT NOT NULL,
		has_poster    INTEGER NOT NULL,
		origin        TEXT NOT NULL,
		search        TEXT NOT NULL,
		created_at    TEXT NOT NULL
	);
	CREATE INDEX items_feed ON items (rank, id);
	CREATE INDEX items_category ON items (category, rank, id);
	CREATE TABLE packs (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL,
		position    INTEGER NOT NULL
	);
	CREATE TABLE pack_items (
		pack_id  TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
		item_id  TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
		position INTEGER NOT NULL,
		PRIMARY KEY (pack_id, item_id)
	);`,

	// Scenes say how they lay out across displays. Scenes loaded before this
	// existed all drew one continuous picture, so they start as "canvas"
	// until the next seed reads their meta.json.
	`ALTER TABLE items ADD COLUMN layout TEXT;
	UPDATE items SET layout = 'canvas' WHERE kind = 'scene';`,
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("catalog: migrate: %w", err)
	}
	var have int
	if err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&have); err != nil {
		return fmt.Errorf("catalog: migrate: %w", err)
	}
	for v := have + 1; v <= len(migrations); v++ {
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v-1]); err != nil {
			tx.Rollback()
			return fmt.Errorf("catalog: migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			v, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("catalog: migration %d: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Slugify turns free text into a lowercase-dashed slug ("Night Sky" →
// "night-sky").
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	return b.String()
}

// Humanize turns a slug or file base name into a title ("low-tide_02" →
// "Low Tide 02").
func Humanize(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' || unicode.IsSpace(r) })
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// SetCategory records the display name for a category slug.
func (db *DB) SetCategory(ctx context.Context, slug, name string) error {
	_, err := db.sql.ExecContext(ctx, `INSERT INTO categories (slug, name) VALUES (?, ?)
		ON CONFLICT(slug) DO UPDATE SET name = excluded.name`, slug, name)
	return err
}

func searchText(it Item) string {
	parts := append([]string{it.Title}, it.Tags...)
	parts = append(parts, it.Category, it.Author)
	return strings.ToLower(strings.Join(parts, "\n"))
}

// UpsertItem inserts the item or replaces every field except created_at,
// which keeps the time the item first entered the catalog.
func (db *DB) UpsertItem(ctx context.Context, it Item) error {
	if it.ID == "" || it.Title == "" || it.Category == "" {
		return errors.New("catalog: item needs id, title and category")
	}
	if it.Tags == nil {
		it.Tags = []string{}
	}
	tags, err := json.Marshal(it.Tags)
	if err != nil {
		return err
	}
	if it.CreatedAt.IsZero() {
		it.CreatedAt = time.Now()
	}
	var source, layout any
	if it.SourceURL != "" {
		source = it.SourceURL
	}
	if it.Layout != "" {
		layout = it.Layout
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A category nobody named yet gets a readable default name.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO categories (slug, name) VALUES (?, ?)`,
		it.Category, Humanize(it.Category)); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO items
		(id, title, kind, width, height, bytes, stored_bytes, category, tags, author, license,
		 source_url, color, rank, original_ext, original_type, has_poster, origin, search, created_at, layout)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		 title=excluded.title, kind=excluded.kind, width=excluded.width, height=excluded.height,
		 bytes=excluded.bytes, stored_bytes=excluded.stored_bytes, category=excluded.category,
		 tags=excluded.tags, author=excluded.author, license=excluded.license,
		 source_url=excluded.source_url, color=excluded.color, rank=excluded.rank,
		 original_ext=excluded.original_ext, original_type=excluded.original_type,
		 has_poster=excluded.has_poster, origin=excluded.origin, search=excluded.search,
		 layout=excluded.layout`,
		it.ID, it.Title, it.Kind, it.Width, it.Height, it.Bytes, it.StoredBytes, it.Category, string(tags),
		it.Author, it.License, source, it.Color, it.Rank, it.OriginalExt, it.OriginalType, it.HasPoster,
		it.Origin, searchText(it), it.CreatedAt.UTC().Truncate(time.Second).Format(time.RFC3339), layout)
	if err != nil {
		return fmt.Errorf("catalog: save %s: %w", it.ID, err)
	}
	return tx.Commit()
}

const itemCols = `id, title, kind, width, height, bytes, stored_bytes, category, tags, author, license,
	source_url, color, rank, original_ext, original_type, has_poster, origin, created_at, layout`

type scanner interface{ Scan(dest ...any) error }

func scanItem(s scanner) (Item, error) {
	var it Item
	var tags, created string
	var source, layout sql.NullString
	err := s.Scan(&it.ID, &it.Title, &it.Kind, &it.Width, &it.Height, &it.Bytes, &it.StoredBytes, &it.Category,
		&tags, &it.Author, &it.License, &source, &it.Color, &it.Rank, &it.OriginalExt, &it.OriginalType,
		&it.HasPoster, &it.Origin, &created, &layout)
	if err != nil {
		return it, err
	}
	it.SourceURL, it.Layout = source.String, layout.String
	if err := json.Unmarshal([]byte(tags), &it.Tags); err != nil || it.Tags == nil {
		it.Tags = []string{}
	}
	it.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return it, nil
}

// GetItem returns one item or ErrNotFound.
func (db *DB) GetItem(ctx context.Context, id string) (Item, error) {
	it, err := scanItem(db.sql.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// DeleteItem removes an item and its pack memberships. ErrNotFound if absent.
func (db *DB) DeleteItem(ctx context.Context, id string) error {
	res, err := db.sql.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountItems is the total number of items.
func (db *DB) CountItems(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&n)
	return n, err
}

// StoredBytes sums the stored size of every item except excludeID (pass ""
// to include all). Callers use it to enforce the storage cap.
func (db *DB) StoredBytes(ctx context.Context, excludeID string) (int64, error) {
	var n int64
	err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(SUM(stored_bytes), 0) FROM items WHERE id <> ?`, excludeID).Scan(&n)
	return n, err
}

// MaxRank is the highest rank in use, or 0 for an empty catalog.
func (db *DB) MaxRank(ctx context.Context) (int64, error) {
	var n int64
	err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(rank), 0) FROM items`).Scan(&n)
	return n, err
}

// ItemsByOrigin lists every item a given source ("seed", "scene", "ingest")
// put in the catalog, in feed order.
func (db *DB) ItemsByOrigin(ctx context.Context, origin string) ([]Item, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+itemCols+` FROM items WHERE origin = ? ORDER BY rank, id`, origin)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

func collect(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func encodeCursor(rank int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(rank, 10) + ":" + id))
}

func decodeCursor(c string) (int64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, "", ErrBadCursor
	}
	rankStr, id, ok := strings.Cut(string(raw), ":")
	rank, err := strconv.ParseInt(rankStr, 10, 64)
	if !ok || err != nil || id == "" {
		return 0, "", ErrBadCursor
	}
	return rank, id, nil
}

// escapeLike makes user text safe inside a LIKE pattern (ESCAPE '\').
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ListItems returns one page in feed order (rank, then id). Paging is by
// position rather than offset, so pages never repeat or skip items even if
// the catalog changes between requests.
func (db *DB) ListItems(ctx context.Context, q Query) (Page, error) {
	where := []string{"1=1"}
	var args []any

	switch q.Kind {
	case "":
	case "live":
		where = append(where, "kind IN ('video','scene')")
	case KindImage, KindVideo, KindScene:
		where = append(where, "kind = ?")
		args = append(args, q.Kind)
	default:
		return Page{}, fmt.Errorf("catalog: unknown kind %q", q.Kind)
	}
	if q.Category != "" {
		where = append(where, "category = ?")
		args = append(args, q.Category)
	}
	if q.Wide {
		where = append(where, "width * 10 >= height * 26") // width/height >= 2.6, in whole numbers
	}
	if s := strings.ToLower(strings.TrimSpace(q.Q)); s != "" {
		pat := "%" + escapeLike(s) + "%"
		where = append(where, `(search LIKE ? ESCAPE '\' OR category IN
			(SELECT slug FROM categories WHERE lower(name) LIKE ? ESCAPE '\'))`)
		args = append(args, pat, pat)
	}
	if q.Cursor != "" {
		rank, id, err := decodeCursor(q.Cursor)
		if err != nil {
			return Page{}, err
		}
		where = append(where, "(rank > ? OR (rank = ? AND id > ?))")
		args = append(args, rank, rank, id)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 30
	}
	args = append(args, limit+1) // one extra row tells us whether another page exists

	rows, err := db.sql.QueryContext(ctx, `SELECT `+itemCols+` FROM items WHERE `+
		strings.Join(where, " AND ")+` ORDER BY rank ASC, id ASC LIMIT ?`, args...)
	if err != nil {
		return Page{}, err
	}
	items, err := collect(rows)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = encodeCursor(last.Rank, last.ID)
	}
	return page, nil
}

// Categories lists categories that have at least one item, ordered by name.
func (db *DB) Categories(ctx context.Context) ([]Category, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT i.category, COALESCE(c.name, i.category) AS name, COUNT(*)
		FROM items i LEFT JOIN categories c ON c.slug = i.category
		GROUP BY i.category ORDER BY name COLLATE NOCASE, i.category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.Slug, &c.Name, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutPack creates or replaces a pack and its ordered membership. Ids that are
// not in the catalog are skipped.
func (db *DB) PutPack(ctx context.Context, p Pack, itemIDs []string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO packs (id, name, description, position) VALUES (?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, description=excluded.description, position=excluded.position`,
		p.ID, p.Name, p.Description, p.Position); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pack_items WHERE pack_id = ?`, p.ID); err != nil {
		return err
	}
	for i, id := range itemIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pack_items (pack_id, item_id, position)
			SELECT ?, id, ? FROM items WHERE id = ?`, p.ID, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePack removes a pack; a missing pack is not an error.
func (db *DB) DeletePack(ctx context.Context, id string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM packs WHERE id = ?`, id)
	return err
}

// Packs lists packs that have at least one item, with counts and cover ids.
func (db *DB) Packs(ctx context.Context) ([]Pack, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT id, name, description, position FROM packs ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	var all []*Pack
	byID := map[string]*Pack{}
	for rows.Next() {
		p := &Pack{CoverIDs: []string{}}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Position); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, p)
		byID[p.ID] = p
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = db.sql.QueryContext(ctx, `SELECT pack_id, item_id FROM pack_items ORDER BY pack_id, position, item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var packID, itemID string
		if err := rows.Scan(&packID, &itemID); err != nil {
			return nil, err
		}
		if p := byID[packID]; p != nil {
			p.Count++
			if len(p.CoverIDs) < 3 {
				p.CoverIDs = append(p.CoverIDs, itemID)
			}
		}
	}
	out := []Pack{}
	for _, p := range all {
		if p.Count > 0 {
			out = append(out, *p)
		}
	}
	return out, rows.Err()
}

// GetPack returns a pack and its items in pack order, or ErrNotFound.
func (db *DB) GetPack(ctx context.Context, id string) (Pack, []Item, error) {
	p := Pack{CoverIDs: []string{}}
	err := db.sql.QueryRowContext(ctx, `SELECT id, name, description, position FROM packs WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil, ErrNotFound
	}
	if err != nil {
		return p, nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+prefixed("i", itemCols)+` FROM pack_items pi
		JOIN items i ON i.id = pi.item_id WHERE pi.pack_id = ? ORDER BY pi.position, i.id`, id)
	if err != nil {
		return p, nil, err
	}
	items, err := collect(rows)
	if err != nil {
		return p, nil, err
	}
	p.Count = len(items)
	for _, it := range items {
		if len(p.CoverIDs) < 3 {
			p.CoverIDs = append(p.CoverIDs, it.ID)
		}
	}
	return p, items, nil
}

// prefixed qualifies a comma-separated column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}
