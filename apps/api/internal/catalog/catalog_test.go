package catalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "sub dir", "catalog.db")) // a space in the path must be fine
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func item(n int, mut func(*Item)) Item {
	it := Item{
		ID: fmt.Sprintf("%012x", n), Title: fmt.Sprintf("Item %d", n), Kind: KindImage,
		Width: 3840, Height: 2400, Bytes: 1000, StoredBytes: 1500, Category: "gradients",
		Tags: []string{"calm"}, Author: "Still Originals", License: "Still Original", Color: "#112233",
		Rank: int64(n) * 100, OriginalExt: "jpg", OriginalType: "image/jpeg", Origin: "seed",
	}
	if mut != nil {
		mut(&it)
	}
	return it
}

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func mustList(t *testing.T, db *DB, q Query) []string {
	t.Helper()
	page, err := db.ListItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return ids(page.Items)
}

func TestFilters(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	if err := db.SetCategory(ctx, "horizons", "Horizons"); err != nil {
		t.Fatal(err)
	}
	seed := []Item{
		item(1, func(it *Item) { it.Title = "Low Tide"; it.Tags = []string{"warm", "coastal"} }),
		item(2, func(it *Item) { it.Title = "Far Ridge"; it.Category = "horizons"; it.Tags = []string{"dusk"} }),
		item(3, func(it *Item) { it.Title = "Slow Dawn"; it.Width = 7680 }), // 3.2:1 panorama
		item(4, func(it *Item) { it.Title = "Clip"; it.Kind = KindVideo; it.Category = "live"; it.HasPoster = true }),
		item(5, func(it *Item) {
			it.Title = "Tidal"
			it.Kind = KindScene
			it.Category = "live"
			it.HasPoster = true
			it.Author = "Ása Nordström"
		}),
		item(6, func(it *Item) { it.Title = "Almost Wide"; it.Width = 6200 }),  // 2.58:1, just under the line
		item(7, func(it *Item) { it.Title = "Exactly Wide"; it.Width = 6240 }), // 2.6:1 exactly
		item(8, func(it *Item) { it.Title = "100% Cotton_Paper"; it.Tags = []string{} }),
	}
	for _, it := range seed {
		if err := db.UpsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	id := func(n int) string { return fmt.Sprintf("%012x", n) }

	cases := []struct {
		name string
		q    Query
		want []string
	}{
		{"everything in rank order", Query{}, []string{id(1), id(2), id(3), id(4), id(5), id(6), id(7), id(8)}},
		{"kind=image", Query{Kind: "image"}, []string{id(1), id(2), id(3), id(6), id(7), id(8)}},
		{"kind=live is video or scene", Query{Kind: "live"}, []string{id(4), id(5)}},
		{"category", Query{Category: "horizons"}, []string{id(2)}},
		{"unknown category is empty", Query{Category: "nope"}, []string{}},
		{"wide means ratio >= 2.6", Query{Wide: true}, []string{id(3), id(7)}},
		{"q matches title, any case", Query{Q: "LOW ti"}, []string{id(1)}},
		{"q matches a tag", Query{Q: "coast"}, []string{id(1)}},
		{"q matches category slug", Query{Q: "horizon"}, []string{id(2)}},
		{"q matches category name", Query{Q: "Horizons"}, []string{id(2)}},
		{"q matches author, non-ASCII, any case", Query{Q: "ÁSA nord"}, []string{id(5)}},
		{"q treats % and _ literally", Query{Q: "100%"}, []string{id(8)}},
		{"q underscore is not a wildcard", Query{Q: "cotton_paper"}, []string{id(8)}},
		{"q wildcard characters alone match nothing else", Query{Q: "_"}, []string{id(8)}},
		{"filters combine", Query{Kind: "image", Wide: true, Q: "dawn"}, []string{id(3)}},
		{"no match", Query{Q: "zzzz"}, []string{}},
	}
	for _, c := range cases {
		if got := mustList(t, db, c.q); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	if _, err := db.ListItems(ctx, Query{Kind: "gif"}); err == nil {
		t.Error("an unknown kind should be an error")
	}
}

func TestCursorPagingHasNoDuplicatesOrGaps(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	// 53 items; many share a rank, so paging has to break ties by id.
	var want []string
	for n := 1; n <= 53; n++ {
		it := item(n, func(it *Item) { it.Rank = int64(n % 7) })
		if err := db.UpsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	all, err := db.ListItems(ctx, Query{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	want = ids(all.Items)
	if len(want) != 53 || all.NextCursor != "" {
		t.Fatalf("full list: %d items, cursor %q", len(want), all.NextCursor)
	}
	for i := 1; i < len(all.Items); i++ {
		a, b := all.Items[i-1], all.Items[i]
		if a.Rank > b.Rank || (a.Rank == b.Rank && a.ID >= b.ID) {
			t.Fatalf("not ordered by rank then id at %d", i)
		}
	}

	for _, limit := range []int{1, 7, 10, 30, 53, 60} {
		var got []string
		cursor := ""
		for pages := 0; ; pages++ {
			if pages > 100 {
				t.Fatalf("limit %d: paging never ends", limit)
			}
			page, err := db.ListItems(ctx, Query{Limit: limit, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) > limit {
				t.Fatalf("limit %d: page has %d items", limit, len(page.Items))
			}
			got = append(got, ids(page.Items)...)
			if page.NextCursor == "" {
				break
			}
			if len(page.Items) != limit {
				t.Fatalf("limit %d: a non-final page has %d items", limit, len(page.Items))
			}
			cursor = page.NextCursor
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d: paged result differs from the full list (%d vs %d items)", limit, len(got), len(want))
		}
	}

	// Paging with a filter, and with an item removed between pages.
	first, _ := db.ListItems(ctx, Query{Limit: 5})
	if err := db.DeleteItem(ctx, first.Items[4].ID); err != nil { // the item the cursor points at
		t.Fatal(err)
	}
	second, err := db.ListItems(ctx, Query{Limit: 5, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(second.Items), want[5:10]) {
		t.Errorf("after deleting the cursor's item, next page = %v, want %v", ids(second.Items), want[5:10])
	}

	// The default limit is 30.
	def, _ := db.ListItems(ctx, Query{})
	if len(def.Items) != 30 || def.NextCursor == "" {
		t.Errorf("default page: %d items, cursor %q", len(def.Items), def.NextCursor)
	}

	for _, bad := range []string{"not-base64!", "aGVsbG8", "OmFiYw"} {
		if _, err := db.ListItems(ctx, Query{Cursor: bad}); !errors.Is(err, ErrBadCursor) {
			t.Errorf("cursor %q: got %v, want ErrBadCursor", bad, err)
		}
	}
}

func TestUpsertGetDelete(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	created := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	orig := item(1, func(it *Item) { it.CreatedAt = created; it.SourceURL = "https://example.com/a"; it.Tags = nil })
	if err := db.UpsertItem(ctx, orig); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetItem(ctx, orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(created) || got.SourceURL != "https://example.com/a" || got.Tags == nil || len(got.Tags) != 0 {
		t.Fatalf("round trip: %+v", got)
	}

	// Updating keeps the original creation time and can clear the source URL.
	changed := orig
	changed.Title, changed.SourceURL, changed.CreatedAt = "Renamed", "", created.Add(48*time.Hour)
	if err := db.UpsertItem(ctx, changed); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetItem(ctx, orig.ID)
	if got.Title != "Renamed" || got.SourceURL != "" || !got.CreatedAt.Equal(created) {
		t.Fatalf("after update: %+v", got)
	}
	if n, _ := db.CountItems(ctx); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	if _, err := db.GetItem(ctx, "ffffffffffff"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing item: got %v", err)
	}
	if err := db.DeleteItem(ctx, orig.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteItem(ctx, orig.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: got %v, want ErrNotFound", err)
	}
}

func TestCategoriesAndStoredBytes(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	db.SetCategory(ctx, "gradients", "Gradients")
	db.SetCategory(ctx, "aurora", "Aurora")
	db.SetCategory(ctx, "empty", "Empty") // has no items, must not be listed
	for n := 1; n <= 3; n++ {
		db.UpsertItem(ctx, item(n, nil))
	}
	db.UpsertItem(ctx, item(4, func(it *Item) { it.Category = "aurora" }))
	db.UpsertItem(ctx, item(5, func(it *Item) { it.Category = "night-sky" })) // unnamed: gets a readable default

	cats, err := db.Categories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Category{{"aurora", "Aurora", 1}, {"gradients", "Gradients", 3}, {"night-sky", "Night Sky", 1}}
	if !slices.Equal(cats, want) {
		t.Fatalf("categories = %+v, want %+v", cats, want)
	}

	if total, _ := db.StoredBytes(ctx, ""); total != 5*1500 {
		t.Errorf("stored bytes = %d", total)
	}
	if others, _ := db.StoredBytes(ctx, fmt.Sprintf("%012x", 2)); others != 4*1500 {
		t.Errorf("stored bytes excluding one = %d", others)
	}
	if top, _ := db.MaxRank(ctx); top != 500 {
		t.Errorf("max rank = %d", top)
	}
}

func TestPacks(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	for n := 1; n <= 5; n++ {
		db.UpsertItem(ctx, item(n, nil))
	}
	id := func(n int) string { return fmt.Sprintf("%012x", n) }

	// Pack order is the order given, not rank order; unknown ids are skipped.
	if err := db.PutPack(ctx, Pack{ID: "b-pack", Name: "B", Description: "Second.", Position: 2},
		[]string{id(5), id(1), "000000000bad", id(3), id(2)}); err != nil {
		t.Fatal(err)
	}
	if err := db.PutPack(ctx, Pack{ID: "a-pack", Name: "A", Description: "First.", Position: 1}, []string{id(4)}); err != nil {
		t.Fatal(err)
	}
	if err := db.PutPack(ctx, Pack{ID: "hollow", Name: "Hollow", Description: "Nothing in it.", Position: 0}, nil); err != nil {
		t.Fatal(err)
	}

	packs, err := db.Packs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 2 || packs[0].ID != "a-pack" || packs[1].ID != "b-pack" {
		t.Fatalf("packs = %+v (empty packs must be hidden, order by position)", packs)
	}
	if packs[1].Count != 4 || !slices.Equal(packs[1].CoverIDs, []string{id(5), id(1), id(3)}) {
		t.Fatalf("b-pack = %+v", packs[1])
	}

	p, items, err := db.GetPack(ctx, "b-pack")
	if err != nil {
		t.Fatal(err)
	}
	if p.Count != 4 || !slices.Equal(ids(items), []string{id(5), id(1), id(3), id(2)}) {
		t.Fatalf("GetPack: %+v %v", p, ids(items))
	}

	// Removing an item drops it from packs; replacing a pack replaces its members.
	db.DeleteItem(ctx, id(1))
	if _, items, _ = db.GetPack(ctx, "b-pack"); !slices.Equal(ids(items), []string{id(5), id(3), id(2)}) {
		t.Fatalf("after deleting an item: %v", ids(items))
	}
	db.PutPack(ctx, Pack{ID: "b-pack", Name: "B2", Description: "Second.", Position: 2}, []string{id(2)})
	if p, items, _ = db.GetPack(ctx, "b-pack"); p.Name != "B2" || !slices.Equal(ids(items), []string{id(2)}) {
		t.Fatalf("after replacing: %+v %v", p, ids(items))
	}

	if _, _, err := db.GetPack(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing pack: got %v", err)
	}
	if err := db.DeletePack(ctx, "a-pack"); err != nil {
		t.Fatal(err)
	}
	if packs, _ = db.Packs(ctx); len(packs) != 1 {
		t.Fatalf("after DeletePack: %+v", packs)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.UpsertItem(context.Background(), item(1, nil))
	db.Close()

	db, err = Open(path) // migrations must not run twice
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.CountItems(context.Background()); n != 1 {
		t.Fatalf("after reopen count = %d", n)
	}
}

func TestSlugifyHumanize(t *testing.T) {
	for in, want := range map[string]string{"Night Sky": "night-sky", "  --A/B__c  ": "a-b-c", "Über": "über", "": ""} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"low-tide_02": "Low Tide 02", "ember field": "Ember Field", "": ""} {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", in, got, want)
		}
	}
}
