package store

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "hc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func addEpisode(t *testing.T, s *Store, show int64, title string, ep int) {
	t.Helper()
	lib, err := s.AddLibrary("/tv", "shows")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.InsertItem(NewItem{LibraryID: lib.ID, Kind: "episode", Title: title, ShowID: show, Season: 1, Episode: ep,
		File: File{Path: filepath.Join("/tv", title, title+string(rune('a'+ep))+".mkv")}})
	if err != nil {
		t.Fatal(err)
	}
}

func showCount(t *testing.T, s *Store) int {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM shows`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A show renamed to its TMDB title must still be found by the folder name.
func TestUpsertShowAfterRename(t *testing.T) {
	s := openTest(t)
	id, err := s.UpsertShow("Vostok Station", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyShowMeta(id, Meta{TmdbID: 42, Title: "Станция «Восток»", Year: 2024, Matched: 1}); err != nil {
		t.Fatal(err)
	}
	for _, y := range []int{0, 2024} {
		got, err := s.UpsertShow("Vostok Station", y)
		if err != nil || got != id {
			t.Fatalf("year %d: got %d, %v; want %d", y, got, err, id)
		}
	}
	if n := showCount(t, s); n != 1 {
		t.Fatalf("%d shows", n)
	}
}

// Matching a duplicate row merges it instead of failing on UNIQUE(title, year).
func TestApplyShowMetaMergesDuplicate(t *testing.T) {
	s := openTest(t)
	a, _ := s.UpsertShow("Vostok Station", 0)
	addEpisode(t, s, a, "Vostok Station", 1)
	if err := s.ApplyShowMeta(a, Meta{TmdbID: 42, Title: "Станция «Восток»", Year: 2024, Matched: 1}); err != nil {
		t.Fatal(err)
	}
	// What older versions did: a second row for the same folder.
	res, err := s.Exec(`INSERT INTO shows(title, year, folder_title) VALUES('Vostok Station', 0, 'Vostok Station')`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := res.LastInsertId()
	addEpisode(t, s, b, "Vostok Station", 2)
	if err := s.ApplyShowMeta(b, Meta{TmdbID: 42, Title: "Станция «Восток»", Year: 2024, Matched: 1}); err != nil {
		t.Fatal(err)
	}
	if n := showCount(t, s); n != 1 {
		t.Fatalf("%d shows", n)
	}
	var eps int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM media_items WHERE show_id=?`, b).Scan(&eps)
	if eps != 2 {
		t.Fatalf("%d episodes on the kept show", eps)
	}
}

// Databases already holding the duplicate are cleaned up on open.
func TestMigrateMergesExistingDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hc.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ra, _ := s.Exec(`INSERT INTO shows(title, year, tmdb_id, matched) VALUES('Станция «Восток»', 2024, 42, 1)`)
	a, _ := ra.LastInsertId()
	rb, _ := s.Exec(`INSERT INTO shows(title, year) VALUES('Vostok Station', 0)`)
	b, _ := rb.LastInsertId()
	addEpisode(t, s, a, "Vostok Station", 1)
	addEpisode(t, s, b, "Vostok Station", 2)
	addEpisode(t, s, b, "Vostok Station", 3)
	if _, err := s.Exec(`INSERT INTO lists(profile_id, list, show_id, added_at) VALUES(1, 'favorites', ?, 0)`, b); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if n := showCount(t, s); n != 1 {
		t.Fatalf("%d shows", n)
	}
	var eps, fav int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM media_items WHERE show_id=?`, a).Scan(&eps)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM lists WHERE show_id=?`, a).Scan(&fav)
	if eps != 3 || fav != 1 {
		t.Fatalf("episodes=%d favorites=%d on the matched show", eps, fav)
	}
	if got, _ := s.UpsertShow("Vostok Station", 0); got != a {
		t.Fatalf("lookup by folder name: got %d, want %d", got, a)
	}
}
