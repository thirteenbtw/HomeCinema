package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeTMDB(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/trending/movie/week", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"page":1,"total_pages":900,"results":[
			{"id":693134,"title":"Дюна: Часть вторая","original_title":"Dune: Part Two","release_date":"2024-02-27","poster_path":"/p.jpg","vote_average":8.16,"genre_ids":[878]},
			{"id":1,"title":"Adult","adult":true}]}`))
	})
	mux.HandleFunc("/search/multi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"page":1,"total_pages":1,"results":[
			{"id":1396,"media_type":"tv","name":"Во все тяжкие","original_name":"Breaking Bad","first_air_date":"2008-01-20"},
			{"id":17419,"media_type":"person","name":"Bryan Cranston"}]}`))
	})
	mux.HandleFunc("/tv/1396", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1396,"name":"Во все тяжкие","original_name":"Breaking Bad","first_air_date":"2008-01-20","overview":"Учитель химии…",
			"genres":[{"name":"драма"}],"episode_run_time":[47],"vote_average":8.9,"poster_path":"/bb.jpg",
			"seasons":[{"season_number":0,"name":"Спецматериалы","episode_count":9},{"season_number":1,"name":"Сезон 1","episode_count":7,"air_date":"2008-01-20"}],
			"credits":{"cast":[{"name":"Bryan Cranston","character":"Walter White"}],"crew":[{"name":"Vince Gilligan","job":"Director"}]},
			"external_ids":{"imdb_id":"tt0903747"},
			"recommendations":{"page":1,"total_pages":1,"results":[{"id":60059,"name":"Лучше звоните Солу"}]}}`))
	})
	mux.HandleFunc("/t/p/w342/p.jpg", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("jpeg")) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldAPI, oldImg := apiBase, imgBase
	apiBase, imgBase = srv.URL, srv.URL+"/t/p/"
	t.Cleanup(func() { apiBase, imgBase = oldAPI, oldImg })
	cacheMu.Lock()
	cache = map[string]cached{}
	cacheMu.Unlock()
	return New(t.TempDir(), func() string { return "key" }, func() string { return "ru-RU" })
}

func TestBrowse(t *testing.T) {
	c := fakeTMDB(t)
	p, err := c.Browse(context.Background(), BrowseQuery{Kind: "movie", List: "trending"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Pages != 500 || len(p.Items) != 1 {
		t.Fatalf("pages=%d items=%d", p.Pages, len(p.Items))
	}
	it := p.Items[0]
	if it.Title != "Дюна: Часть вторая" || it.Orig != "Dune: Part Two" || it.Year != 2024 || it.Rating != 8.2 || it.Poster != "/api/catalog-img/w342/p.jpg" {
		t.Fatalf("%+v", it)
	}
}

func TestSearchCatalogSkipsPeople(t *testing.T) {
	c := fakeTMDB(t)
	p, err := c.SearchCatalog(context.Background(), "во все", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].Kind != "tv" || p.Items[0].Orig != "Breaking Bad" || p.Items[0].Year != 2008 {
		t.Fatalf("%+v", p.Items)
	}
}

func TestTitle(t *testing.T) {
	c := fakeTMDB(t)
	x, err := c.Title(context.Background(), "tv", 1396)
	if err != nil {
		t.Fatal(err)
	}
	if x.Title != "Во все тяжкие" || x.Orig != "Breaking Bad" || x.Runtime != 47 || x.ImdbID != "tt0903747" {
		t.Fatalf("%+v", x)
	}
	if len(x.Seasons) != 1 || x.Seasons[0].Number != 1 || x.Seasons[0].Year != 2008 {
		t.Fatalf("seasons %+v", x.Seasons)
	}
	if len(x.Cast) != 2 || x.Cast[0].Role != "Режиссёр" || len(x.Related) != 1 || x.Related[0].Kind != "tv" {
		t.Fatalf("cast %+v related %+v", x.Cast, x.Related)
	}
	if len(x.Genres) != 1 || x.Genres[0] != "Драма" {
		t.Fatalf("genres %+v", x.Genres)
	}
}

func TestCatalogImage(t *testing.T) {
	c := fakeTMDB(t)
	if _, err := c.CatalogImage(context.Background(), "w342", "/../../etc/passwd"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := c.CatalogImage(context.Background(), "original", "/p.jpg"); err == nil {
		t.Fatal("unknown size accepted")
	}
	name, err := c.CatalogImage(context.Background(), "w342", "/p.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(c.ImageDir, name)); string(b) != "jpeg" {
		t.Fatalf("cached %q", b)
	}
}

func TestMirror(t *testing.T) {
	c := fakeTMDB(t)
	real := apiBase
	apiBase = "http://127.0.0.1:1" // TMDB "blocked"
	c.Mirrors = func() []Mirror { return []Mirror{{API: real + "/", Image: imgBase}} }
	if _, err := c.Browse(context.Background(), BrowseQuery{Kind: "movie", List: "trending"}); err != nil {
		t.Fatalf("mirror not used: %v", err)
	}
	c.Mirrors = nil
	cacheMu.Lock()
	cache = map[string]cached{}
	cacheMu.Unlock()
	_, err := c.Browse(context.Background(), BrowseQuery{Kind: "movie", List: "trending"})
	if err == nil || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("want a connection error without the key, got %v", err)
	}
}

func TestMirrorFallback(t *testing.T) {
	c := fakeTMDB(t)
	real := apiBase
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer broken.Close()
	calls := 0
	c.Mirrors = func() []Mirror {
		calls++
		return []Mirror{{API: "http://127.0.0.1:1/3"}, {API: broken.URL}, {API: real}}
	}
	if _, err := c.Browse(context.Background(), BrowseQuery{Kind: "movie", List: "trending"}); err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if c.good.Load() != 2 {
		t.Fatalf("working mirror not remembered: %d", c.good.Load())
	}
	if first := c.order()[0]; first.API != real {
		t.Fatalf("order starts with %+v", first)
	}
}
