package records

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "records.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

var noon = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func TestNormalizeEmail(t *testing.T) {
	good := map[string]string{
		"  Ana.Ruiz@Example.COM ": "ana.ruiz@example.com",
		"a+tag@mail.example.org":  "a+tag@mail.example.org",
		"o'neil@example.ie":       "o'neil@example.ie",
		"x@b.co":                  "x@b.co",
		"jörg@münchen.example":    "jörg@münchen.example",
	}
	for in, want := range good {
		if got, ok := NormalizeEmail(in); !ok || got != want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	bad := []string{
		"", "plain", "@example.com", "a@", "a@b", "a@b.c", "a@localhost", "a b@example.com", "a@exa mple.com",
		"a@@example.com", ".a@example.com", "a.@example.com", "a..b@example.com", "a@-example.com", "a@example-.com",
		"a@example..com", "a@example.com.", "a@127.0.0.1", "Ana <ana@example.com>", "a@example.com\nbcc: x@y.zz",
		`"quoted"@example.com`, "a,b@example.com", strings.Repeat("a", 65) + "@example.com",
		"a@" + strings.Repeat("b", 250) + ".com",
	}
	for _, in := range bad {
		if got, ok := NormalizeEmail(in); ok {
			t.Errorf("NormalizeEmail(%q) accepted as %q", in, got)
		}
	}
}

func TestSignups(t *testing.T) {
	ctx := context.Background()
	db := open(t)

	for i, email := range []string{"ana@example.com", "ben@example.com", "cy@example.com"} {
		added, err := db.AddSignup(ctx, email, SourceSite, "IN", "", noon.Add(time.Duration(i)*24*time.Hour))
		if err != nil || !added {
			t.Fatalf("AddSignup(%s) = %v, %v", email, added, err)
		}
	}
	// The same address again changes nothing and is not an error.
	if added, err := db.AddSignup(ctx, "ana@example.com", SourceManual, "", "again", noon); err != nil || added {
		t.Fatalf("duplicate AddSignup = %v, %v", added, err)
	}

	all, err := db.ListSignups(ctx, SignupQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Email != "cy@example.com" || all[2].Email != "ana@example.com" {
		t.Fatalf("list = %+v, want three, newest first", all)
	}
	ana := all[2]
	if ana.Source != SourceSite || ana.Country != "IN" || ana.Status != StatusNew || !ana.CreatedAt.Equal(noon) || !ana.InvitedAt.IsZero() {
		t.Errorf("stored sign-up = %+v", ana)
	}

	// Marking as sent stamps the time once; doing it again changes nothing.
	sent := noon.Add(72 * time.Hour)
	if n, err := db.SetStatus(ctx, []int64{ana.ID, all[1].ID, 9999}, StatusInvited, sent); err != nil || n != 2 {
		t.Fatalf("SetStatus = %d, %v; want 2", n, err)
	}
	if n, _ := db.SetStatus(ctx, []int64{ana.ID}, StatusInvited, sent.Add(time.Hour)); n != 0 {
		t.Errorf("marking twice changed %d rows", n)
	}
	if _, err := db.SetStatus(ctx, []int64{ana.ID}, "banned", sent); err == nil {
		t.Error("an unknown status was accepted")
	}
	invited, _ := db.ListSignups(ctx, SignupQuery{Status: StatusInvited})
	if len(invited) != 2 || !invited[0].InvitedAt.Equal(sent) {
		t.Errorf("invited = %+v", invited)
	}
	if n, _ := db.SetStatus(ctx, []int64{ana.ID}, StatusNew, sent); n != 1 {
		t.Errorf("un-marking changed %d rows", n)
	}
	back, _ := db.ListSignups(ctx, SignupQuery{Search: "ANA@"})
	if len(back) != 1 || back[0].Status != StatusNew || !back[0].InvitedAt.IsZero() {
		t.Errorf("after un-marking = %+v", back)
	}

	if err := db.SetNote(ctx, ana.ID, "Friend from college"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNote(ctx, 9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetNote on a missing id = %v", err)
	}
	// Search covers notes, and % is text, not a wildcard.
	if got, _ := db.ListSignups(ctx, SignupQuery{Search: "college"}); len(got) != 1 {
		t.Errorf("search by note found %d", len(got))
	}
	if got, _ := db.ListSignups(ctx, SignupQuery{Search: "%"}); len(got) != 0 {
		t.Errorf("searching for %% matched %d rows", len(got))
	}
	if got, _ := db.ListSignups(ctx, SignupQuery{Limit: 2}); len(got) != 2 {
		t.Errorf("limit 2 returned %d", len(got))
	}

	totals, _ := db.SignupTotals(ctx)
	if totals.Total != 3 || totals.Invited != 1 {
		t.Errorf("totals = %+v", totals)
	}
	byDay, _ := db.SignupsByDay(ctx, "2026-11-11")
	if len(byDay) != 2 || byDay["2026-11-11"] != 1 || byDay["2026-11-12"] != 1 {
		t.Errorf("by day = %v", byDay)
	}

	if err := db.DeleteSignup(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSignup(ctx, ana.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice = %v", err)
	}
	if totals, _ := db.SignupTotals(ctx); totals.Total != 2 {
		t.Errorf("after delete, total = %d", totals.Total)
	}
}

func TestCounts(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	yesterday := noon.Add(-24 * time.Hour)
	for range 3 {
		db.Count(ctx, noon, CountFile, "aaaaaaaaaaaa")
	}
	db.Count(ctx, noon, CountFile, "bbbbbbbbbbbb")
	db.Count(ctx, yesterday, CountFile, "bbbbbbbbbbbb")
	db.Count(ctx, yesterday, CountVisit, "")
	// 23:30 in New York on the 9th is already the 10th in UTC.
	db.Count(ctx, time.Date(2026, 11, 9, 23, 30, 0, 0, time.FixedZone("EST", -5*3600)), CountVisit, "")

	files, _ := db.CountsByDay(ctx, CountFile, "2026-11-01")
	if files["2026-11-10"] != 4 || files["2026-11-09"] != 1 {
		t.Errorf("files by day = %v", files)
	}
	visits, _ := db.CountsByDay(ctx, CountVisit, "2026-11-10")
	if len(visits) != 1 || visits["2026-11-10"] != 1 {
		t.Errorf("visits from the 10th = %v", visits)
	}
	top, _ := db.CountsBySubject(ctx, CountFile, "")
	if len(top) != 2 || top[0] != (SubjectCount{"aaaaaaaaaaaa", 3}) || top[1] != (SubjectCount{"bbbbbbbbbbbb", 2}) {
		t.Errorf("by subject, all time = %v", top)
	}
	today, _ := db.CountsBySubject(ctx, CountFile, "2026-11-10")
	if len(today) != 2 || today[1].N != 1 {
		t.Errorf("by subject, today = %v", today)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	if err := db.CreateSession(ctx, "hash-one", noon, noon.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		hash string
		at   time.Time
		want bool
	}{
		{"hash-one", noon.Add(30 * time.Minute), true},
		{"hash-one", noon.Add(time.Hour), false}, // run out
		{"hash-two", noon, false},
	} {
		if ok, err := db.SessionValid(ctx, c.hash, c.at); err != nil || ok != c.want {
			t.Errorf("SessionValid(%s at +%v) = %v, %v", c.hash, c.at.Sub(noon), ok, err)
		}
	}
	// Starting a later session clears the one that has run out.
	db.CreateSession(ctx, "hash-two", noon.Add(2*time.Hour), noon.Add(3*time.Hour))
	var n int
	db.sql.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 1 {
		t.Errorf("%d sessions stored, want only the live one", n)
	}
	db.DeleteSession(ctx, "hash-two")
	if ok, _ := db.SessionValid(ctx, "hash-two", noon.Add(2*time.Hour)); ok {
		t.Error("a deleted session is still valid")
	}
}

func TestLogKeepsNewestFirst(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	db.Log(ctx, noon, "signed-in", "")
	db.Log(ctx, noon.Add(time.Minute), "marked", "2 people marked as sent")
	events, err := db.RecentEvents(ctx, 10)
	if err != nil || len(events) != 2 || events[0].Action != "marked" || events[1].Action != "signed-in" {
		t.Fatalf("events = %+v, %v", events, err)
	}
}

func TestFileIsPrivateAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "records.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.AddSignup(context.Background(), "ana@example.com", SourceSite, "", "", noon)
	db.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("records.db mode = %v, want 0600", info.Mode().Perm())
	}
	again, err := Open(path) // migrations must not run twice
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if totals, _ := again.SignupTotals(context.Background()); totals.Total != 1 {
		t.Errorf("after reopening, total = %d", totals.Total)
	}
}
