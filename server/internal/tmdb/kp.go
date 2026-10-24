package tmdb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// Kinopoisk (kinopoiskapiunofficial.tech) is the default metadata provider:
// unlike TMDB it is reachable from Russia without a VPN, and it has Russian
// titles and descriptions for nearly everything. Every public method of
// Client switches to it when Provider() is "kinopoisk"; ids stored in the
// library are then Kinopoisk ids.

var kpBase = "https://kinopoiskapiunofficial.tech"

var ErrNoKPKey error = noKPKey{}

// noKPKey also matches ErrNoKey, so callers that wait for a key stop either way.
type noKPKey struct{}

func (noKPKey) Error() string {
	return "нужен ключ API Кинопоиска — получите его бесплатно на kinopoiskapiunofficial.tech и добавьте в настройках"
}
func (noKPKey) Is(target error) bool { return target == ErrNoKey }

func (c *Client) kp() bool { return c.Provider != nil && c.Provider() == "kinopoisk" }

// ProviderName is shown to the user next to ratings and links.
func (c *Client) ProviderName() string {
	if c.kp() {
		return "Кинопоиск"
	}
	return "TMDB"
}

func (c *Client) kpGet(ctx context.Context, path string, q url.Values, out any) error {
	key := ""
	if c.KPKey != nil {
		key = strings.TrimSpace(c.KPKey())
	}
	if key == "" {
		return ErrNoKPKey
	}
	u := kpBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		req.Header.Set("X-API-KEY", key)
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return fmt.Errorf("нет связи с Кинопоиском: %v", err)
		}
		if resp.StatusCode == 429 && attempt < 3 {
			resp.Body.Close()
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case 200:
			return json.NewDecoder(resp.Body).Decode(out)
		case 401:
			return fmt.Errorf("Кинопоиск: неверный ключ")
		case 402:
			return fmt.Errorf("Кинопоиск: дневной лимит запросов ключа исчерпан, попробуйте завтра")
		case 404:
			return fmt.Errorf("Кинопоиск: не найдено")
		case 429:
			return fmt.Errorf("Кинопоиск: слишком много запросов")
		}
		return fmt.Errorf("Кинопоиск %s: %s", path, resp.Status)
	}
}

// kpCached is kpGet behind the shared response cache.
func (c *Client) kpCached(ctx context.Context, path string, q url.Values, out any) error {
	return c.cachedCall("kp"+path+"?"+q.Encode(), out, func(raw *json.RawMessage) error { return c.kpGet(ctx, path, q, raw) })
}

type kpGenre struct {
	Genre string `json:"genre"`
}
type kpCountry struct {
	Country string `json:"country"`
}

// kpFilm covers the film objects of every Kinopoisk endpoint (they differ
// in id field names and in how year and rating are typed).
type kpFilm struct {
	KinopoiskID   int64       `json:"kinopoiskId"`
	FilmID        int64       `json:"filmId"`
	NameRu        string      `json:"nameRu"`
	NameEn        string      `json:"nameEn"`
	NameOriginal  string      `json:"nameOriginal"`
	Year          flexNum     `json:"year"`
	Type          string      `json:"type"`
	Poster        string      `json:"posterUrl"`
	PosterPreview string      `json:"posterUrlPreview"`
	Cover         string      `json:"coverUrl"`
	Description   string      `json:"description"`
	RatingKP      flexNum     `json:"ratingKinopoisk"`
	RatingImdb    flexNum     `json:"ratingImdb"`
	Rating        flexNum     `json:"rating"`
	Votes         flexNum     `json:"ratingVoteCount"`
	VotesKP       flexNum     `json:"ratingKinopoiskVoteCount"`
	Genres        []kpGenre   `json:"genres"`
	Countries     []kpCountry `json:"countries"`
	Length        flexNum     `json:"filmLength"`
	Slogan        string      `json:"slogan"`
	AgeLimit      string      `json:"ratingAgeLimits"`
	ImdbID        string      `json:"imdbId"`
	Serial        bool        `json:"serial"`
}

// flexNum accepts 7.5, "7.5", "1:30" (minutes) and null.
type flexNum float64

func (n *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" || strings.HasSuffix(s, "%") {
		*n = 0
		return nil
	}
	if h, m, ok := strings.Cut(s, ":"); ok { // film length "1:52"
		hh, _ := strconv.Atoi(h)
		mm, _ := strconv.Atoi(m)
		*n = flexNum(hh*60 + mm)
		return nil
	}
	v, _ := strconv.ParseFloat(s, 64)
	*n = flexNum(v)
	return nil
}

func (f kpFilm) id() int64 {
	if f.KinopoiskID != 0 {
		return f.KinopoiskID
	}
	return f.FilmID
}

func (f kpFilm) kind() string {
	switch f.Type {
	case "TV_SERIES", "MINI_SERIES", "TV_SHOW":
		return "tv"
	}
	if f.Serial {
		return "tv"
	}
	return "movie"
}

func (f kpFilm) title() string {
	for _, s := range []string{f.NameRu, f.NameOriginal, f.NameEn} {
		if s != "" {
			return s
		}
	}
	return ""
}

func (f kpFilm) orig() string {
	if f.NameOriginal != "" {
		return f.NameOriginal
	}
	return f.NameEn
}

func (f kpFilm) rating() float64 {
	for _, r := range []flexNum{f.RatingKP, f.Rating, f.RatingImdb} {
		if r > 0 {
			return round1(float64(r))
		}
	}
	return 0
}

func (f kpFilm) item() CatalogItem {
	poster := f.PosterPreview
	if poster == "" {
		poster = f.Poster
	}
	return CatalogItem{Kind: f.kind(), ID: f.id(), Title: f.title(), Orig: f.orig(), Year: int(f.Year),
		Poster: proxyURL(poster), Backdrop: proxyURL(f.Cover), Overview: f.Description, Rating: f.rating(),
		Votes: int(max(f.Votes, f.VotesKP)), Genres: []int{}}
}

// proxyURL routes an external image through the server's caching proxy.
func proxyURL(raw string) string {
	if raw == "" {
		return ""
	}
	return "/api/catalog-img/u/" + base64.RawURLEncoding.EncodeToString([]byte(raw)) + ".jpg"
}

var kpImageHosts = map[string]bool{"kinopoiskapiunofficial.tech": true, "avatars.mds.yandex.net": true, "st.kp.yandex.net": true, "image.tmdb.org": true}

// decodeProxyURL validates a proxied image address.
func decodeProxyURL(enc string) (string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSuffix(enc, ".jpg"))
	if err != nil {
		return "", false
	}
	u, err := url.Parse(string(b))
	if err != nil || u.Scheme != "https" || !kpImageHosts[u.Hostname()] || u.User != nil {
		return "", false
	}
	return u.String(), true
}

func kpTypeFor(kind string) string {
	if kind == "tv" {
		return "TV_SERIES"
	}
	return "FILM"
}

// ---------- library matching ----------

func (c *Client) kpSearch(ctx context.Context, kind, query string, year int) ([]SearchResult, error) {
	var r struct {
		Films []kpFilm `json:"films"`
	}
	if err := c.kpCached(ctx, "/api/v2.1/films/search-by-keyword", url.Values{"keyword": {query}}, &r); err != nil {
		return nil, err
	}
	out := []SearchResult{}
	for _, f := range r.Films {
		if kind != "" && f.kind() != kind {
			continue
		}
		sr := SearchResult{ID: f.id(), Title: f.title(), Original: f.orig(), Poster: f.PosterPreview, Overview: f.Description, Votes: int(f.Votes)}
		if f.Year > 0 {
			sr.Release = strconv.Itoa(int(f.Year))
		}
		out = append(out, sr)
	}
	return out, nil
}

func (c *Client) kpFilm(ctx context.Context, id int64) (kpFilm, error) {
	var f kpFilm
	err := c.kpCached(ctx, fmt.Sprintf("/api/v2.2/films/%d", id), nil, &f)
	return f, err
}

type kpStaff struct {
	ID          int64  `json:"staffId"`
	NameRu      string `json:"nameRu"`
	NameEn      string `json:"nameEn"`
	Description string `json:"description"`
	Photo       string `json:"posterUrl"`
	Profession  string `json:"professionKey"`
}

func (s kpStaff) name() string {
	if s.NameRu != "" {
		return s.NameRu
	}
	return s.NameEn
}

func (c *Client) kpStaff(ctx context.Context, id int64) []kpStaff {
	var st []kpStaff
	_ = c.kpCached(ctx, "/api/v1/staff", url.Values{"filmId": {strconv.FormatInt(id, 10)}}, &st)
	return st
}

func ageCert(s string) string {
	if v := strings.TrimPrefix(s, "age"); v != s && v != "" {
		return v + "+"
	}
	return ""
}

func kpGenres(gs []kpGenre) []string {
	out := []string{}
	for _, g := range gs {
		if g.Genre != "" {
			out = append(out, capitalize(g.Genre))
		}
	}
	return out
}

func (c *Client) kpDetails(ctx context.Context, id int64) (store.Meta, error) {
	f, err := c.kpFilm(ctx, id)
	if err != nil {
		return store.Meta{}, err
	}
	m := store.Meta{TmdbID: f.id(), Title: f.title(), OrigTitle: f.orig(), Year: int(f.Year), Overview: f.Description,
		Rating: f.rating(), Runtime: int(f.Length), Genres: kpGenres(f.Genres), Cert: ageCert(f.AgeLimit), Matched: 1}
	m.Poster = c.cacheURL(ctx, f.Poster)
	m.Backdrop = c.cacheURL(ctx, f.Cover)
	actors := 0
	for _, s := range c.kpStaff(ctx, id) {
		switch {
		case s.Profession == "DIRECTOR" && len(m.Cast) == actors:
			m.Cast = append(m.Cast, store.CastMember{Name: s.name(), Role: "Режиссёр", Photo: c.cacheURL(ctx, s.Photo)})
		case s.Profession == "ACTOR" && actors < 12:
			actors++
			m.Cast = append(m.Cast, store.CastMember{Name: s.name(), Role: s.Description, Photo: c.cacheURL(ctx, s.Photo)})
		}
	}
	return m, nil
}

type kpSeasons struct {
	Items []struct {
		Number   int `json:"number"`
		Episodes []struct {
			Season   int    `json:"seasonNumber"`
			Episode  int    `json:"episodeNumber"`
			NameRu   string `json:"nameRu"`
			NameEn   string `json:"nameEn"`
			Synopsis string `json:"synopsis"`
			Release  string `json:"releaseDate"`
		} `json:"episodes"`
	} `json:"items"`
}

func (c *Client) kpEpisode(ctx context.Context, showID int64, season, episode int) (store.Meta, error) {
	var s kpSeasons
	if err := c.kpCached(ctx, fmt.Sprintf("/api/v2.2/films/%d/seasons", showID), nil, &s); err != nil {
		return store.Meta{}, err
	}
	for _, se := range s.Items {
		for _, e := range se.Episodes {
			if e.Season == season && e.Episode == episode {
				name := e.NameRu
				if name == "" {
					name = e.NameEn
				}
				return store.Meta{EpTitle: name, Overview: e.Synopsis, Matched: 1}, nil
			}
		}
	}
	return store.Meta{}, fmt.Errorf("серия S%02dE%02d не найдена", season, episode)
}

// ---------- catalog ----------

// kpLists are Kinopoisk collections shown as catalog rows.
var kpLists = map[string][]List{
	"movie": {
		{"TOP_POPULAR_MOVIES", "Популярные фильмы"}, {"CLOSES_RELEASES", "Премьеры"}, {"TOP_250_MOVIES", "Топ-250 Кинопоиска"},
		{"FAMILY", "Для всей семьи"}, {"KIDS_ANIMATION_THEME", "Мультфильмы"}, {"COMICS_THEME", "По комиксам"},
	},
	"tv": {
		{"POPULAR_SERIES", "Популярные сериалы"}, {"TOP_250_TV_SHOWS", "Топ-250 сериалов"},
	},
}

func (c *Client) kpBrowse(ctx context.Context, q BrowseQuery) (CatalogPage, error) {
	var r struct {
		Pages int      `json:"totalPages"`
		Items []kpFilm `json:"items"`
	}
	page := strconv.Itoa(max(q.Page, 1))
	isList := false
	for _, l := range kpLists[q.Kind] {
		isList = isList || l.Key == q.List
	}
	if isList && q.Genre == 0 && q.Year == 0 {
		if err := c.kpCached(ctx, "/api/v2.2/films/collections", url.Values{"type": {q.List}, "page": {page}}, &r); err != nil {
			return CatalogPage{}, err
		}
	} else {
		vals := url.Values{"type": {kpTypeFor(q.Kind)}, "page": {page}, "order": {"NUM_VOTE"}}
		switch q.Sort {
		case "rating":
			vals.Set("order", "RATING")
			vals.Set("ratingFrom", "7")
		case "date":
			vals.Set("order", "YEAR")
			vals.Set("yearTo", strconv.Itoa(time.Now().Year()))
		}
		if q.Genre > 0 {
			vals.Set("genres", strconv.Itoa(q.Genre))
		}
		if q.Year > 0 {
			vals.Set("yearFrom", strconv.Itoa(q.Year))
			vals.Set("yearTo", strconv.Itoa(q.Year))
		}
		if err := c.kpCached(ctx, "/api/v2.2/films", vals, &r); err != nil {
			return CatalogPage{}, err
		}
	}
	p := CatalogPage{Page: max(q.Page, 1), Pages: max(r.Pages, 1), Items: []CatalogItem{}}
	for _, f := range r.Items {
		p.Items = append(p.Items, f.item())
	}
	return p, nil
}

func (c *Client) kpSearchCatalog(ctx context.Context, query string, page int) (CatalogPage, error) {
	var r struct {
		Pages int      `json:"pagesCount"`
		Films []kpFilm `json:"films"`
	}
	if err := c.kpCached(ctx, "/api/v2.1/films/search-by-keyword", url.Values{"keyword": {query}, "page": {strconv.Itoa(max(page, 1))}}, &r); err != nil {
		return CatalogPage{}, err
	}
	p := CatalogPage{Page: max(page, 1), Pages: max(r.Pages, 1), Items: []CatalogItem{}}
	for _, f := range r.Films {
		if f.Type == "VIDEO" && f.Description == "" {
			continue
		}
		p.Items = append(p.Items, f.item())
	}
	return p, nil
}

func (c *Client) kpGenreList(ctx context.Context) ([]Genre, error) {
	var r struct {
		Genres []struct {
			ID    int    `json:"id"`
			Genre string `json:"genre"`
		} `json:"genres"`
	}
	if err := c.kpCached(ctx, "/api/v2.2/films/filters", nil, &r); err != nil {
		return nil, err
	}
	out := []Genre{}
	for _, g := range r.Genres {
		switch g.Genre {
		case "", "для взрослых", "церемония", "концерт", "реальное ТВ", "ток-шоу", "новости", "игра":
			continue
		}
		out = append(out, Genre{ID: g.ID, Name: capitalize(g.Genre)})
	}
	return out, nil
}

func (c *Client) kpTitle(ctx context.Context, kind string, id int64) (CatalogTitle, error) {
	f, err := c.kpFilm(ctx, id)
	if err != nil {
		return CatalogTitle{}, err
	}
	it := f.item()
	t := CatalogTitle{CatalogItem: it, Genres: kpGenres(f.Genres), Runtime: int(f.Length), Tagline: f.Slogan,
		Cert: ageCert(f.AgeLimit), ImdbID: f.ImdbID, Cast: []CatalogCast{}, Related: []CatalogItem{}}
	t.Poster = proxyURL(f.Poster)
	for _, cn := range f.Countries {
		t.Countries = append(t.Countries, cn.Country)
	}
	actors := 0
	for _, s := range c.kpStaff(ctx, id) {
		switch {
		case s.Profession == "DIRECTOR" && len(t.Cast) == actors:
			t.Cast = append(t.Cast, CatalogCast{Name: s.name(), Role: "Режиссёр", Photo: proxyURL(s.Photo)})
		case s.Profession == "ACTOR" && actors < 15:
			actors++
			t.Cast = append(t.Cast, CatalogCast{Name: s.name(), Role: s.Description, Photo: proxyURL(s.Photo)})
		}
	}
	if t.Kind == "tv" {
		var s kpSeasons
		if c.kpCached(ctx, fmt.Sprintf("/api/v2.2/films/%d/seasons", id), nil, &s) == nil {
			for _, se := range s.Items {
				y := 0
				if len(se.Episodes) > 0 {
					y = yearOf(se.Episodes[0].Release)
				}
				t.Seasons = append(t.Seasons, Season{Number: se.Number, Name: fmt.Sprintf("Сезон %d", se.Number), Episodes: len(se.Episodes), Year: y})
			}
		}
	}
	var sim struct {
		Items []kpFilm `json:"items"`
	}
	if c.kpCached(ctx, fmt.Sprintf("/api/v2.2/films/%d/similars", id), nil, &sim) == nil {
		for i, s := range sim.Items {
			if i >= 18 {
				break
			}
			ci := s.item()
			ci.Kind = t.Kind // similars carry no type; they are nearly always the same kind
			t.Related = append(t.Related, ci)
		}
	}
	return t, nil
}
