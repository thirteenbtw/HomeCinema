package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeKP(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-KEY") != "kpkey" {
				w.WriteHeader(401)
				return
			}
			f(w, r)
		}
	}
	mux.HandleFunc("/api/v2.2/films/collections", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "TOP_POPULAR_MOVIES" {
			t.Errorf("collection %q", r.URL.Query().Get("type"))
		}
		w.Write([]byte(`{"total":2,"totalPages":35,"items":[
			{"kinopoiskId":1048334,"nameRu":"Дюна: Часть вторая","nameEn":null,"nameOriginal":"Dune: Part Two","ratingKinopoisk":8.3,"ratingImdb":8.5,"year":2024,"type":"FILM","posterUrl":"https://kinopoiskapiunofficial.tech/images/posters/kp/1048334.jpg","posterUrlPreview":"https://kinopoiskapiunofficial.tech/images/posters/kp_small/1048334.jpg","coverUrl":null,"genres":[{"genre":"фантастика"}]},
			{"kinopoiskId":404900,"nameRu":"Во все тяжкие","nameOriginal":"Breaking Bad","ratingKinopoisk":null,"ratingImdb":9.5,"year":2008,"type":"TV_SERIES","posterUrlPreview":""}]}`))
	}))
	mux.HandleFunc("/api/v2.2/films", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("genres") != "6" || q.Get("type") != "TV_SERIES" || q.Get("order") != "RATING" {
			t.Errorf("discover %v", q)
		}
		w.Write([]byte(`{"total":1,"totalPages":1,"items":[{"kinopoiskId":404900,"nameRu":"Во все тяжкие","year":2008,"type":"TV_SERIES","ratingKinopoisk":8.9}]}`))
	}))
	mux.HandleFunc("/api/v2.1/films/search-by-keyword", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"keyword":"kill bill","pagesCount":1,"films":[
			{"filmId":5492,"nameRu":"Убить Билла","nameEn":"Kill Bill: Vol. 1","type":"FILM","year":"2003","filmLength":"01:51","rating":"8.0","ratingVoteCount":400000,"posterUrlPreview":"https://kinopoiskapiunofficial.tech/images/posters/kp_small/5492.jpg"},
			{"filmId":5493,"nameRu":"Убить Билла 2","nameEn":"Kill Bill: Vol. 2","type":"FILM","year":"2004","rating":"7.9"},
			{"filmId":9,"nameRu":"Убить Билла: сериал","type":"TV_SERIES","year":"2030","rating":"null"}]}`))
	}))
	mux.HandleFunc("/api/v2.2/films/5492", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"kinopoiskId":5492,"imdbId":"tt0266697","nameRu":"Убить Билла","nameOriginal":"Kill Bill: Vol. 1","year":2003,"filmLength":111,
			"description":"Невеста мстит.","slogan":"Месть — блюдо…","ratingKinopoisk":8.0,"ratingAgeLimits":"age18","type":"FILM",
			"posterUrl":"https://kinopoiskapiunofficial.tech/images/posters/kp/5492.jpg","coverUrl":"https://avatars.mds.yandex.net/get-ott/1/2/orig",
			"genres":[{"genre":"боевик"},{"genre":"криминал"}],"countries":[{"country":"США"}]}`))
	}))
	mux.HandleFunc("/api/v2.2/films/301", auth(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"kinopoiskId":301}`)) }))
	mux.HandleFunc("/api/v2.2/films/404900", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"kinopoiskId":404900,"nameRu":"Во все тяжкие","nameOriginal":"Breaking Bad","year":2008,"type":"TV_SERIES","serial":true,"filmLength":47}`))
	}))
	mux.HandleFunc("/api/v2.2/films/404900/seasons", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"items":[{"number":1,"episodes":[{"seasonNumber":1,"episodeNumber":1,"nameRu":"Пилот","nameEn":"Pilot","synopsis":"Уолтер…","releaseDate":"2008-01-20"},{"seasonNumber":1,"episodeNumber":2,"nameRu":null,"nameEn":"Cat's in the Bag...","releaseDate":"2008-01-27"}]}]}`))
	}))
	mux.HandleFunc("/api/v2.2/films/5492/similars", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"items":[{"filmId":5493,"nameRu":"Убить Билла 2","posterUrlPreview":"https://kinopoiskapiunofficial.tech/images/posters/kp_small/5493.jpg"}]}`))
	}))
	mux.HandleFunc("/api/v2.2/films/404900/similars", auth(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"total":0,"items":[]}`)) }))
	mux.HandleFunc("/api/v1/staff", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"staffId":1,"nameRu":"Квентин Тарантино","description":null,"posterUrl":"https://kinopoiskapiunofficial.tech/images/actor_posters/kp/1.jpg","professionKey":"DIRECTOR"},
			{"staffId":2,"nameRu":"Ума Турман","description":"Невеста","professionKey":"ACTOR"},
			{"staffId":3,"nameRu":"Лоуренс Бендер","professionKey":"PRODUCER"}]`))
	}))
	mux.HandleFunc("/api/v2.2/films/filters", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"genres":[{"id":1,"genre":"триллер"},{"id":2,"genre":""},{"id":6,"genre":"драма"},{"id":20,"genre":"для взрослых"}],"countries":[]}`))
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := kpBase
	kpBase = srv.URL
	t.Cleanup(func() { kpBase = old })
	cacheMu.Lock()
	cache = map[string]cached{}
	cacheMu.Unlock()
	c := New(t.TempDir(), func() string { return "" }, func() string { return "ru-RU" })
	c.Provider = func() string { return "kinopoisk" }
	c.KPKey = func() string { return "kpkey" }
	return c
}

func TestKPBrowse(t *testing.T) {
	c := fakeKP(t)
	p, err := c.Browse(context.Background(), BrowseQuery{Kind: "movie", List: "TOP_POPULAR_MOVIES"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Pages != 35 || len(p.Items) != 2 {
		t.Fatalf("%+v", p)
	}
	d := p.Items[0]
	if d.ID != 1048334 || d.Title != "Дюна: Часть вторая" || d.Orig != "Dune: Part Two" || d.Year != 2024 || d.Rating != 8.3 || d.Kind != "movie" || !strings.HasPrefix(d.Poster, "/api/catalog-img/u/") {
		t.Fatalf("%+v", d)
	}
	if bb := p.Items[1]; bb.Kind != "tv" || bb.Rating != 9.5 || bb.Poster != "" {
		t.Fatalf("%+v", bb)
	}
	raw, ok := decodeProxyURL(strings.TrimPrefix(d.Poster, "/api/catalog-img/u/"))
	if !ok || raw != "https://kinopoiskapiunofficial.tech/images/posters/kp_small/1048334.jpg" {
		t.Fatalf("proxy %q %v", raw, ok)
	}
	p, err = c.Browse(context.Background(), BrowseQuery{Kind: "tv", Genre: 6, Sort: "rating"})
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("discover %v %+v", err, p)
	}
}

func TestKPProxyRejectsOtherHosts(t *testing.T) {
	for _, raw := range []string{"https://evil.example/x.jpg", "http://kinopoiskapiunofficial.tech/x.jpg", "file:///etc/passwd", "https://user@avatars.mds.yandex.net/x"} {
		enc := strings.TrimPrefix(proxyURL(raw), "/api/catalog-img/u/")
		if _, ok := decodeProxyURL(enc); ok {
			t.Errorf("%s accepted", raw)
		}
	}
}

func TestKPMatching(t *testing.T) {
	c := fakeKP(t)
	res, err := c.Search(context.Background(), "movie", "Kill Bill Vol 1", 2003)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("tv result not filtered: %+v", res)
	}
	best := BestMatch(res, "Kill Bill Vol 1", 2003)
	if best == nil || best.ID != 5492 {
		t.Fatalf("best %+v", best)
	}
	m, err := c.Details(context.Background(), "movie", best.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Убить Билла" || m.OrigTitle != "Kill Bill: Vol. 1" || m.Runtime != 111 || m.Cert != "18+" || m.Rating != 8 || len(m.Genres) != 2 {
		t.Fatalf("%+v", m)
	}
	if len(m.Cast) != 2 || m.Cast[0].Role != "Режиссёр" || m.Cast[1].Name != "Ума Турман" {
		t.Fatalf("cast %+v", m.Cast)
	}
	e, err := c.Episode(context.Background(), 404900, 1, 2)
	if err != nil || e.EpTitle != "Cat's in the Bag..." {
		t.Fatalf("episode %+v %v", e, err)
	}
	if _, err := c.Episode(context.Background(), 404900, 3, 1); err == nil {
		t.Fatal("missing episode found")
	}
}

func TestKPTitle(t *testing.T) {
	c := fakeKP(t)
	x, err := c.Title(context.Background(), "movie", 5492)
	if err != nil {
		t.Fatal(err)
	}
	if x.Tagline != "Месть — блюдо…" || x.ImdbID != "tt0266697" || len(x.Countries) != 1 || len(x.Related) != 1 || x.Related[0].ID != 5493 || x.Backdrop == "" {
		t.Fatalf("%+v", x)
	}
	s, err := c.Title(context.Background(), "tv", 404900)
	if err != nil || s.Kind != "tv" || len(s.Seasons) != 1 || s.Seasons[0].Episodes != 2 || s.Seasons[0].Year != 2008 {
		t.Fatalf("%+v %v", s, err)
	}
	g, err := c.Genres(context.Background(), "movie")
	if err != nil || len(g) != 2 || g[0].Name != "Триллер" {
		t.Fatalf("genres %+v %v", g, err)
	}
}

func TestKPKeyErrors(t *testing.T) {
	c := fakeKP(t)
	c.KPKey = func() string { return "" }
	if _, err := c.Browse(context.Background(), BrowseQuery{List: "TOP_POPULAR_MOVIES"}); !errors.Is(err, ErrNoKey) || !errors.Is(err, ErrNoKPKey) {
		t.Fatalf("want no-key error, got %v", err)
	}
	c.KPKey = func() string { return "wrong" }
	if err := c.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "неверный ключ") {
		t.Fatalf("got %v", err)
	}
}

func TestFlexNum(t *testing.T) {
	var v struct{ A, B, C, D, E flexNum }
	if err := json.Unmarshal([]byte(`{"A":7.5,"B":"8.1","C":"01:51","D":null,"E":"99%"}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 7.5 || v.B != 8.1 || v.C != 111 || v.D != 0 || v.E != 0 {
		t.Fatalf("%+v", v)
	}
}
