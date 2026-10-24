package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/tmdb"
)

// A folder named differently from the TMDB title in the metadata language.
func fakeScanner(t *testing.T) *Scanner {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"id":42,"name":"Станция «Восток»","original_name":"Vostok Station","first_air_date":"2024-03-01","vote_count":100}]}`))
	})
	mux.HandleFunc("/tv/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":42,"name":"Станция «Восток»","original_name":"Vostok Station","first_air_date":"2024-03-01","overview":"Антарктида."}`))
	})
	mux.HandleFunc("/tv/42/season/1/episode/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"Серия","overview":"…"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	st, err := store.Open(filepath.Join(t.TempDir(), "hc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	c := tmdb.New(t.TempDir(), func() string { return "key" }, func() string { return "ru-RU" })
	c.Mirrors = func() []tmdb.Mirror { return []tmdb.Mirror{{API: srv.URL, Image: srv.URL + "/img"}} }
	return New(st, nil, c, events.New())
}

func addEpisode(t *testing.T, s *store.Store, ep int) {
	t.Helper()
	lib, err := s.AddLibrary("/tv", "shows")
	if err != nil {
		t.Fatal(err)
	}
	show, err := s.UpsertShow("Vostok Station", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.InsertItem(store.NewItem{LibraryID: lib.ID, Kind: "episode", Title: "Vostok Station", ShowID: show, Season: 1, Episode: ep,
		File: store.File{Path: filepath.Join("/tv/Vostok Station", "e"+string(rune('0'+ep))+".mkv")}})
	if err != nil {
		t.Fatal(err)
	}
}

func matchWithin(t *testing.T, sc *Scanner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sc.matchPending(ctx)
	if ctx.Err() != nil {
		t.Fatal("matchPending did not finish")
	}
}

// Episodes indexed after the show got its TMDB title land on the same show.
func TestEpisodesAfterShowMatch(t *testing.T) {
	sc := fakeScanner(t)
	addEpisode(t, sc.Store, 1)
	matchWithin(t, sc)
	addEpisode(t, sc.Store, 2)
	addEpisode(t, sc.Store, 3)
	matchWithin(t, sc)

	var shows, matched int
	var title string
	_ = sc.Store.DB.QueryRow(`SELECT COUNT(*), MAX(title) FROM shows`).Scan(&shows, &title)
	_ = sc.Store.DB.QueryRow(`SELECT COUNT(*) FROM media_items WHERE matched=1`).Scan(&matched)
	if shows != 1 || title != "Станция «Восток»" || matched != 3 {
		t.Fatalf("shows=%d title=%q matched episodes=%d", shows, title, matched)
	}
}

// A row that cannot be saved is parked instead of being retried forever.
func TestMatchPendingSkipsUnsavableRow(t *testing.T) {
	sc := fakeScanner(t)
	// A different show already owns the TMDB title and the folder name.
	if _, err := sc.Store.Exec(`INSERT INTO shows(title, year, tmdb_id, matched, folder_title) VALUES('Станция «Восток»', 2024, 7, 1, 'x')`); err != nil {
		t.Fatal(err)
	}
	res, err := sc.Store.Exec(`INSERT INTO shows(title, year, folder_title, folder_year) VALUES('Vostok', 0, 'Станция «Восток»', 2024)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	matchWithin(t, sc)
	var m int
	_ = sc.Store.DB.QueryRow(`SELECT matched FROM shows WHERE id=?`, id).Scan(&m)
	if m != 2 {
		t.Fatalf("matched=%d, want 2", m)
	}
}
