// Package tmdb fetches posters, descriptions, genres and cast from The Movie
// Database (FR-3) and caches images locally so the library works offline (FR-4).
package tmdb

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/store"
)

var apiBase = "https://api.themoviedb.org/3"
var imgBase = "https://image.tmdb.org/t/p/"

// Mirror is an alternative address for the TMDB API and images, for
// networks where TMDB itself is blocked. Empty fields mean TMDB directly.
type Mirror struct {
	API   string // e.g. https://example.org/3
	Image string // e.g. https://example.org/t/p/
}

var ErrNoKey = errors.New("ключ источника данных не задан")

type Client struct {
	Key      func() string // v3 api key or v4 read access token
	Lang     func() string // e.g. ru-RU
	ImageDir string
	Mirror   func() Mirror // optional
	Provider func() string // "kinopoisk" or "tmdb"
	KPKey    func() string // Kinopoisk API key
	http     *http.Client
}

func New(imageDir string, key, lang func() string) *Client {
	return &Client{Key: key, Lang: lang, ImageDir: imageDir, http: &http.Client{Timeout: 20 * time.Second, Transport: newTransport()}}
}

func (c *Client) bases() (api, img string) {
	api, img = apiBase, imgBase
	if c.Mirror != nil {
		m := c.Mirror()
		if v := strings.TrimRight(strings.TrimSpace(m.API), "/"); v != "" {
			api = v
		}
		if v := strings.TrimRight(strings.TrimSpace(m.Image), "/"); v != "" {
			img = v + "/"
		}
	}
	return api, img
}

// netErr describes a connection failure without the request URL, which
// carries the API key.
func netErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return fmt.Errorf("нет связи с TMDB: %v. Если TMDB заблокирован у провайдера, включите VPN или укажите зеркало TMDB в настройках", err)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	key := strings.TrimSpace(c.Key())
	if key == "" {
		return ErrNoKey
	}
	if q == nil {
		q = url.Values{}
	}
	base, _ := c.bases()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if len(key) > 40 { // v4 token
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		q.Set("api_key", key)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	for attempt := 0; ; attempt++ {
		resp, err := c.http.Do(req)
		if err != nil {
			return netErr(err)
		}
		if resp.StatusCode == 429 && attempt < 3 {
			resp.Body.Close()
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			time.Sleep(time.Duration(max(wait, 1)) * time.Second)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 401 {
			return fmt.Errorf("TMDB: неверный ключ")
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("TMDB %s: %s", path, resp.Status)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
}

// Check verifies that the configured key works.
func (c *Client) Check(ctx context.Context) error {
	if c.kp() {
		var f kpFilm
		return c.kpGet(ctx, "/api/v2.2/films/301", nil, &f)
	}
	var v map[string]any
	return c.get(ctx, "/configuration", nil, &v)
}

type SearchResult struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Name     string  `json:"name"`
	Original string  `json:"original_title"`
	OrigName string  `json:"original_name"`
	Release  string  `json:"release_date"`
	FirstAir string  `json:"first_air_date"`
	Poster   string  `json:"poster_path"`
	Overview string  `json:"overview"`
	Pop      float64 `json:"popularity"`
	Votes    int     `json:"vote_count"`
}

func (r SearchResult) DisplayTitle() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

func (r SearchResult) Year() int {
	d := r.Release
	if d == "" {
		d = r.FirstAir
	}
	if len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		return y
	}
	return 0
}

// Search returns candidates for kind "movie" or "tv". Used by the matcher and
// by the "fix wrong match" dialog.
func (c *Client) Search(ctx context.Context, kind, query string, year int) ([]SearchResult, error) {
	if c.kp() {
		return c.kpSearch(ctx, kind, query, year)
	}
	q := url.Values{"query": {query}, "language": {c.Lang()}, "include_adult": {"false"}}
	if year > 0 {
		if kind == "movie" {
			q.Set("year", strconv.Itoa(year))
		} else {
			q.Set("first_air_date_year", strconv.Itoa(year))
		}
	}
	var out struct {
		Results []SearchResult `json:"results"`
	}
	if err := c.get(ctx, "/search/"+kind, q, &out); err != nil {
		return nil, err
	}
	if len(out.Results) == 0 && year > 0 {
		// release years in file names are often off by one
		return c.Search(ctx, kind, query, 0)
	}
	for i := range out.Results {
		if out.Results[i].Poster != "" {
			_, img := c.bases()
			out.Results[i].Poster = img + "w185" + out.Results[i].Poster
		}
	}
	return out.Results, nil
}

// BestMatch picks the result whose title and year fit the parsed name best.
func BestMatch(results []SearchResult, title string, year int) *SearchResult {
	var best *SearchResult
	bestScore := -1.0
	norm := normalize(title)
	for i := range results {
		r := &results[i]
		score := 0.0
		if normalize(r.DisplayTitle()) == norm || normalize(r.Original+r.OrigName) == norm {
			score += 10
		}
		if year > 0 {
			switch d := r.Year() - year; {
			case d == 0:
				score += 8
			case d == 1 || d == -1:
				score += 4
			}
		}
		score += float64(min(r.Votes, 5000)) / 1000
		score -= float64(i) * 0.5
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	return best
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r == 'ё' {
			r = 'е'
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 'а' && r <= 'я' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type genre struct {
	Name string `json:"name"`
}
type credit struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Job       string `json:"job"`
	Profile   string `json:"profile_path"`
}

type details struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Name        string  `json:"name"`
	Original    string  `json:"original_title"`
	OrigName    string  `json:"original_name"`
	Overview    string  `json:"overview"`
	Poster      string  `json:"poster_path"`
	Backdrop    string  `json:"backdrop_path"`
	Genres      []genre `json:"genres"`
	Vote        float64 `json:"vote_average"`
	Runtime     int     `json:"runtime"`
	Release     string  `json:"release_date"`
	FirstAir    string  `json:"first_air_date"`
	EpisodeRuns []int   `json:"episode_run_time"`
	Credits     struct {
		Cast []credit `json:"cast"`
		Crew []credit `json:"crew"`
	} `json:"credits"`
	ReleaseDates struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Cert string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

// Details loads full metadata for a movie or a show and caches its images.
func (c *Client) Details(ctx context.Context, kind string, id int64) (store.Meta, error) {
	if c.kp() {
		return c.kpDetails(ctx, id)
	}
	var d details
	extra := "credits,release_dates"
	if kind == "tv" {
		extra = "credits,content_ratings"
	}
	if err := c.get(ctx, fmt.Sprintf("/%s/%d", kind, id), url.Values{"language": {c.Lang()}, "append_to_response": {extra}}, &d); err != nil {
		return store.Meta{}, err
	}
	if d.Overview == "" && !strings.HasPrefix(c.Lang(), "en") {
		// No translation: show the original description (the UI marks it).
		var en details
		if c.get(ctx, fmt.Sprintf("/%s/%d", kind, id), url.Values{"language": {"en-US"}}, &en) == nil {
			d.Overview = en.Overview
		}
	}
	m := store.Meta{TmdbID: d.ID, Overview: d.Overview, Rating: round1(d.Vote), Runtime: d.Runtime, Matched: 1}
	m.Title = d.Title
	m.OrigTitle = d.Original
	date := d.Release
	if kind == "tv" {
		m.Title, m.OrigTitle, date = d.Name, d.OrigName, d.FirstAir
		if len(d.EpisodeRuns) > 0 {
			m.Runtime = d.EpisodeRuns[0]
		}
	}
	if len(date) >= 4 {
		m.Year, _ = strconv.Atoi(date[:4])
	}
	for _, g := range d.Genres {
		m.Genres = append(m.Genres, capitalize(g.Name))
	}
	m.Poster = c.cacheImage(ctx, "w500", d.Poster)
	m.Backdrop = c.cacheImage(ctx, "w1280", d.Backdrop)
	for i, cr := range d.Credits.Cast {
		if i >= 12 {
			break
		}
		m.Cast = append(m.Cast, store.CastMember{Name: cr.Name, Role: cr.Character, Photo: c.cacheImage(ctx, "w185", cr.Profile)})
	}
	for _, cr := range d.Credits.Crew {
		if cr.Job == "Director" {
			m.Cast = append(m.Cast, store.CastMember{Name: cr.Name, Role: "Режиссёр", Photo: c.cacheImage(ctx, "w185", cr.Profile)})
			break
		}
	}
	m.Cert = cert(d)
	return m, nil
}

// Episode loads an episode title and description.
func (c *Client) Episode(ctx context.Context, showID int64, season, episode int) (store.Meta, error) {
	if c.kp() {
		return c.kpEpisode(ctx, showID, season, episode)
	}
	var e struct {
		Name     string  `json:"name"`
		Overview string  `json:"overview"`
		Still    string  `json:"still_path"`
		Vote     float64 `json:"vote_average"`
		Runtime  int     `json:"runtime"`
	}
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d/episode/%d", showID, season, episode), url.Values{"language": {c.Lang()}}, &e); err != nil {
		return store.Meta{}, err
	}
	return store.Meta{EpTitle: e.Name, Overview: e.Overview, Backdrop: c.cacheImage(ctx, "w780", e.Still), Rating: round1(e.Vote), Runtime: e.Runtime, Matched: 1}, nil
}

func cert(d details) string {
	pick := func(country string) string {
		for _, r := range d.ReleaseDates.Results {
			if r.Country == country {
				for _, x := range r.Dates {
					if x.Cert != "" {
						return x.Cert
					}
				}
			}
		}
		for _, r := range d.ContentRatings.Results {
			if r.Country == country && r.Rating != "" {
				return r.Rating
			}
		}
		return ""
	}
	if c := pick("RU"); c != "" {
		return c
	}
	return pick("US")
}

// cacheImage downloads an image once and returns its local URL.
func (c *Client) cacheImage(ctx context.Context, size, path string) string {
	if path == "" {
		return ""
	}
	h := sha1.Sum([]byte(size + path))
	_, img := c.bases()
	return c.fetchImage(ctx, img+size+path, hex.EncodeToString(h[:8])+filepath.Ext(path))
}

// cacheURL caches an image given by its full address (Kinopoisk).
func (c *Client) cacheURL(ctx context.Context, raw string) string {
	if raw == "" {
		return ""
	}
	h := sha1.Sum([]byte(raw))
	ext := filepath.Ext(raw)
	if len(ext) > 5 || ext == "" {
		ext = ".jpg"
	}
	return c.fetchImage(ctx, raw, hex.EncodeToString(h[:8])+ext)
}

func (c *Client) fetchImage(ctx context.Context, src, name string) string {
	dst := filepath.Join(c.ImageDir, name)
	local := "/api/images/" + name
	if _, err := os.Stat(dst); err == nil {
		return local
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", src, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return ""
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return ""
	}
	if os.Rename(tmp, dst) != nil {
		return ""
	}
	return local
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
