package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The catalog browses TMDB beyond the local library (trending, popular, by
// genre), so a title can be found first and then fetched from the user's own
// Torznab indexers. Artwork goes through the server (/api/catalog-img) so the
// client never talks to TMDB directly.

type CatalogItem struct {
	Kind     string  `json:"kind"` // movie | tv
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Orig     string  `json:"origTitle"`
	Year     int     `json:"year"`
	Poster   string  `json:"poster"`
	Backdrop string  `json:"backdrop"`
	Overview string  `json:"overview"`
	Rating   float64 `json:"rating"`
	Votes    int     `json:"votes"`
	Genres   []int   `json:"genreIds"`
}

type CatalogPage struct {
	Items []CatalogItem `json:"items"`
	Page  int           `json:"page"`
	Pages int           `json:"pages"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type CatalogTitle struct {
	CatalogItem
	Genres    []string      `json:"genres"`
	Runtime   int           `json:"runtime"`
	Tagline   string        `json:"tagline"`
	Cert      string        `json:"cert"`
	ImdbID    string        `json:"imdbId"`
	Seasons   []Season      `json:"seasons,omitempty"`
	Cast      []CatalogCast `json:"cast"`
	Related   []CatalogItem `json:"related"`
	Countries []string      `json:"countries"`
	Trailer   string        `json:"trailer,omitempty"` // YouTube video key
}

type Season struct {
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Episodes int    `json:"episodes"`
	Year     int    `json:"year"`
}

type CatalogCast struct {
	ID    int64  `json:"id,omitempty"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Photo string `json:"photo"`
}

type listItem struct {
	ID        int64   `json:"id"`
	MediaType string  `json:"media_type"`
	Title     string  `json:"title"`
	Name      string  `json:"name"`
	Original  string  `json:"original_title"`
	OrigName  string  `json:"original_name"`
	Release   string  `json:"release_date"`
	FirstAir  string  `json:"first_air_date"`
	Poster    string  `json:"poster_path"`
	Backdrop  string  `json:"backdrop_path"`
	Overview  string  `json:"overview"`
	Vote      float64 `json:"vote_average"`
	Votes     int     `json:"vote_count"`
	GenreIDs  []int   `json:"genre_ids"`
	Adult     bool    `json:"adult"`
}

type listResp struct {
	Page    int        `json:"page"`
	Pages   int        `json:"total_pages"`
	Results []listItem `json:"results"`
}

// ImageURL maps a TMDB image path to the server's caching proxy.
func ImageURL(size, path string) string {
	if path == "" {
		return ""
	}
	return "/api/catalog-img/" + size + path
}

func (li listItem) toItem(kind string) CatalogItem {
	if li.MediaType != "" {
		kind = li.MediaType
	}
	it := CatalogItem{Kind: kind, ID: li.ID, Title: li.Title, Orig: li.Original, Overview: li.Overview,
		Rating: round1(li.Vote), Votes: li.Votes, Genres: li.GenreIDs,
		Poster: ImageURL("w342", li.Poster), Backdrop: ImageURL("w780", li.Backdrop)}
	date := li.Release
	if kind == "tv" {
		it.Title, it.Orig, date = li.Name, li.OrigName, li.FirstAir
	}
	if it.Genres == nil {
		it.Genres = []int{}
	}
	it.Year = yearOf(date)
	return it
}

func yearOf(date string) int {
	if len(date) >= 4 {
		y, _ := strconv.Atoi(date[:4])
		return y
	}
	return 0
}

func toPage(r listResp, kind string) CatalogPage {
	p := CatalogPage{Page: r.Page, Pages: min(r.Pages, 500), Items: []CatalogItem{}} // TMDB serves at most 500 pages
	for _, li := range r.Results {
		if li.Adult || li.MediaType == "person" {
			continue
		}
		p.Items = append(p.Items, li.toItem(kind))
	}
	return p
}

// Lists that the catalog can show, per kind.
var lists = map[string]map[string]string{
	"movie": {"trending": "/trending/movie/week", "popular": "/movie/popular", "top_rated": "/movie/top_rated", "now_playing": "/movie/now_playing", "upcoming": "/movie/upcoming"},
	"tv":    {"trending": "/trending/tv/week", "popular": "/tv/popular", "top_rated": "/tv/top_rated", "on_the_air": "/tv/on_the_air", "airing_today": "/tv/airing_today"},
}

type BrowseQuery struct {
	Kind  string // movie | tv
	List  string // a key of lists, or "discover"
	Genre int
	Year  int
	Sort  string // popularity | rating | date
	Page  int
}

// Browse returns one page of a TMDB list or of a filtered discover query.
func (c *Client) Browse(ctx context.Context, q BrowseQuery) (CatalogPage, error) {
	if q.Kind != "tv" {
		q.Kind = "movie"
	}
	vals := url.Values{"language": {c.Lang()}, "page": {strconv.Itoa(max(q.Page, 1))}, "region": {region(c.Lang())}}
	path, ok := lists[q.Kind][q.List]
	if !ok || q.Genre > 0 || q.Year > 0 {
		path = "/discover/" + q.Kind
		sort := "popularity.desc"
		switch q.Sort {
		case "rating":
			sort = "vote_average.desc"
			vals.Set("vote_count.gte", "300")
		case "date":
			if q.Kind == "tv" {
				sort = "first_air_date.desc"
			} else {
				sort = "primary_release_date.desc"
			}
			vals.Set("vote_count.gte", "20")
		}
		vals.Set("sort_by", sort)
		vals.Set("include_adult", "false")
		if q.Genre > 0 {
			vals.Set("with_genres", strconv.Itoa(q.Genre))
		}
		if q.Year > 0 {
			if q.Kind == "tv" {
				vals.Set("first_air_date_year", strconv.Itoa(q.Year))
			} else {
				vals.Set("primary_release_year", strconv.Itoa(q.Year))
			}
		}
		if q.Sort == "date" {
			// only titles already out
			today := time.Now().Format("2006-01-02")
			if q.Kind == "tv" {
				vals.Set("first_air_date.lte", today)
			} else {
				vals.Set("primary_release_date.lte", today)
			}
		}
	}
	var r listResp
	if err := c.getCached(ctx, path, vals, &r); err != nil {
		return CatalogPage{}, err
	}
	return toPage(r, q.Kind), nil
}

// SearchCatalog finds movies and shows by title across all of TMDB.
func (c *Client) SearchCatalog(ctx context.Context, query string, page int) (CatalogPage, error) {
	vals := url.Values{"query": {query}, "language": {c.Lang()}, "page": {strconv.Itoa(max(page, 1))}, "include_adult": {"false"}}
	var r listResp
	if err := c.getCached(ctx, "/search/multi", vals, &r); err != nil {
		return CatalogPage{}, err
	}
	return toPage(r, "movie"), nil
}

// Genres lists TMDB genres for a kind in the metadata language.
func (c *Client) Genres(ctx context.Context, kind string) ([]Genre, error) {
	if kind != "tv" {
		kind = "movie"
	}
	var r struct {
		Genres []Genre `json:"genres"`
	}
	if err := c.getCached(ctx, "/genre/"+kind+"/list", url.Values{"language": {c.Lang()}}, &r); err != nil {
		return nil, err
	}
	for i := range r.Genres {
		r.Genres[i].Name = capitalize(r.Genres[i].Name)
	}
	if r.Genres == nil {
		r.Genres = []Genre{}
	}
	return r.Genres, nil
}

// Title loads full catalog details for a movie or a show.
func (c *Client) Title(ctx context.Context, kind string, id int64) (CatalogTitle, error) {
	if kind != "tv" {
		kind = "movie"
	}
	extra := "credits,release_dates,recommendations,external_ids,videos"
	if kind == "tv" {
		extra = "credits,content_ratings,recommendations,external_ids,videos"
	}
	var d struct {
		details
		Tagline   string `json:"tagline"`
		ImdbID    string `json:"imdb_id"`
		Countries []struct {
			Name string `json:"name"`
		} `json:"production_countries"`
		Seasons []struct {
			Number   int    `json:"season_number"`
			Name     string `json:"name"`
			Episodes int    `json:"episode_count"`
			AirDate  string `json:"air_date"`
		} `json:"seasons"`
		External struct {
			ImdbID string `json:"imdb_id"`
		} `json:"external_ids"`
		Recommendations listResp  `json:"recommendations"`
		Videos          videoList `json:"videos"`
	}
	if err := c.getCached(ctx, fmt.Sprintf("/%s/%d", kind, id), url.Values{"language": {c.Lang()}, "append_to_response": {extra}, "include_video_language": {videoLangs(c.Lang())}}, &d); err != nil {
		return CatalogTitle{}, err
	}
	li := listItem{ID: d.details.ID, Title: d.details.Title, Name: d.details.Name, Original: d.details.Original, OrigName: d.details.OrigName,
		Release: d.details.Release, FirstAir: d.details.FirstAir, Poster: d.details.Poster, Backdrop: d.details.Backdrop,
		Overview: d.details.Overview, Vote: d.details.Vote}
	t := CatalogTitle{CatalogItem: li.toItem(kind), Runtime: d.Runtime, Tagline: d.Tagline, Cert: cert(d.details), ImdbID: d.ImdbID}
	t.Poster = ImageURL("w500", d.details.Poster)
	t.Backdrop = ImageURL("w1280", d.details.Backdrop)
	if t.ImdbID == "" {
		t.ImdbID = d.External.ImdbID
	}
	if kind == "tv" && len(d.EpisodeRuns) > 0 {
		t.Runtime = d.EpisodeRuns[0]
	}
	if t.Overview == "" && !strings.HasPrefix(c.Lang(), "en") {
		var en details
		if c.getCached(ctx, fmt.Sprintf("/%s/%d", kind, id), url.Values{"language": {"en-US"}}, &en) == nil {
			t.Overview = en.Overview
		}
	}
	t.Genres = []string{}
	for _, g := range d.Genres {
		t.Genres = append(t.Genres, capitalize(g.Name))
	}
	for _, cn := range d.Countries {
		t.Countries = append(t.Countries, cn.Name)
	}
	t.Cast = []CatalogCast{}
	for i, cr := range d.Credits.Cast {
		if i >= 15 {
			break
		}
		t.Cast = append(t.Cast, CatalogCast{ID: cr.ID, Name: cr.Name, Role: cr.Character, Photo: ImageURL("w185", cr.Profile)})
	}
	for _, cr := range d.Credits.Crew {
		if cr.Job == "Director" {
			t.Cast = append([]CatalogCast{{ID: cr.ID, Name: cr.Name, Role: "Режиссёр", Photo: ImageURL("w185", cr.Profile)}}, t.Cast...)
			break
		}
	}
	for _, s := range d.Seasons {
		if s.Number == 0 && len(d.Seasons) > 1 {
			continue // specials
		}
		t.Seasons = append(t.Seasons, Season{Number: s.Number, Name: s.Name, Episodes: s.Episodes, Year: yearOf(s.AirDate)})
	}
	t.Trailer = d.Videos.trailer(c.Lang())
	t.Related = toPage(d.Recommendations, kind).Items
	if len(t.Related) > 18 {
		t.Related = t.Related[:18]
	}
	return t, nil
}

var reImage = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|jpeg|png|webp)$`)
var imageSizes = map[string]bool{"w185": true, "w342": true, "w500": true, "w780": true, "w1280": true}

// CatalogImage downloads (once) and returns the local file name of a TMDB
// image requested through the catalog proxy.
func (c *Client) CatalogImage(ctx context.Context, size, path string) (string, error) {
	if !imageSizes[size] || !reImage.MatchString(path) {
		return "", fmt.Errorf("bad image")
	}
	local := c.cacheImage(ctx, size, path)
	if local == "" {
		return "", fmt.Errorf("image unavailable")
	}
	return strings.TrimPrefix(local, "/api/images/"), nil
}

func region(lang string) string {
	if i := strings.IndexByte(lang, '-'); i >= 0 {
		return lang[i+1:]
	}
	return "RU"
}

// Short-lived response cache: browsing the catalog hits the same lists again
// and again, and TMDB rate-limits.
type cached struct {
	body []byte
	at   time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cached{}
)

const cacheTTL = 30 * time.Minute

func (c *Client) getCached(ctx context.Context, path string, q url.Values, out any) error {
	key := path + "?" + q.Encode()
	cacheMu.Lock()
	e, ok := cache[key]
	cacheMu.Unlock()
	if ok && time.Since(e.at) < cacheTTL {
		return json.Unmarshal(e.body, out)
	}
	var raw json.RawMessage
	if err := c.get(ctx, path, q, &raw); err != nil {
		return err
	}
	cacheMu.Lock()
	if len(cache) > 2000 {
		for k, v := range cache {
			if time.Since(v.at) >= cacheTTL {
				delete(cache, k)
			}
		}
		if len(cache) > 2000 {
			cache = map[string]cached{}
		}
	}
	cache[key] = cached{body: raw, at: time.Now()}
	cacheMu.Unlock()
	return json.Unmarshal(raw, out)
}
