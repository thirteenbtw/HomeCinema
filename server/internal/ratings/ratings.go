// Package ratings fetches the IMDb and Letterboxd scores of a title for the
// catalog page. Neither site has an open API, so each score has a few
// sources tried at once; the first one to answer wins.
package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Score struct {
	Value float64 `json:"value"` // IMDb: out of 10; Letterboxd: out of 5
	Votes int     `json:"votes,omitempty"`
	URL   string  `json:"url"`
	From  string  `json:"from,omitempty"` // which source answered
}

type Result struct {
	IMDb       *Score `json:"imdb"`
	Letterboxd *Score `json:"letterboxd"`
}

// Hosts, replaced in tests.
var (
	imdbJSONP  = "https://p.media-imdb.com"
	cinemeta   = "https://v3-cinemeta.strem.io"
	imdbSite   = "https://www.imdb.com"
	letterboxd = "https://letterboxd.com"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

type Client struct {
	http *http.Client
	mu   sync.Mutex
	memo map[string]memo
}

type memo struct {
	r  Result
	at time.Time
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 12 * time.Second}, memo: map[string]memo{}}
}

// Get returns the scores of a TMDB title. imdbID may be empty (then there is
// no IMDb score); Letterboxd only rates films. Missing scores are nil.
func (c *Client) Get(ctx context.Context, kind string, tmdbID int64, imdbID string) Result {
	key := fmt.Sprintf("%s:%d", kind, tmdbID)
	c.mu.Lock()
	m, ok := c.memo[key]
	c.mu.Unlock()
	// a found score is good for a day, a miss is retried sooner
	if ok && (time.Since(m.at) < 30*time.Minute || m.r.IMDb != nil && m.r.Letterboxd != nil && time.Since(m.at) < 24*time.Hour) {
		return m.r
	}
	var r Result
	var wg sync.WaitGroup
	if imdbID != "" {
		wg.Add(1)
		go func() { defer wg.Done(); r.IMDb = c.imdb(ctx, kind, imdbID) }()
	}
	if kind == "movie" {
		wg.Add(1)
		go func() { defer wg.Done(); r.Letterboxd = c.letterboxd(ctx, tmdbID) }()
	}
	wg.Wait()
	if ctx.Err() == nil {
		c.mu.Lock()
		if len(c.memo) > 5000 {
			c.memo = map[string]memo{}
		}
		c.memo[key] = memo{r, time.Now()}
		c.mu.Unlock()
	}
	return r
}

func (c *Client) fetch(ctx context.Context, u string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), resp.Request.URL.String(), err
}

// first runs the sources at once and returns the first score found.
func first(ctx context.Context, srcs ...func(context.Context) (*Score, error)) *Score {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan *Score, len(srcs))
	for _, f := range srcs {
		go func() {
			s, err := f(ctx)
			if err != nil || s == nil || s.Value <= 0 {
				s = nil
			}
			ch <- s
		}()
	}
	for range srcs {
		if s := <-ch; s != nil {
			return s
		}
	}
	return nil
}

var errNoScore = errors.New("no score")

// ---------- IMDb ----------

func (c *Client) imdb(ctx context.Context, kind, id string) *Score {
	page := "https://www.imdb.com/title/" + id + "/"
	s := first(ctx,
		// the JSONP feed IMDb's own widgets use
		func(ctx context.Context) (*Score, error) {
			body, _, err := c.fetch(ctx, imdbJSONP+"/static-content/documents/v1/title/"+id+"/ratings%3Fjsonp=imdb.rating.run:imdb.rating.run.title/data.json")
			if err != nil {
				return nil, err
			}
			return parseIMDbJSONP(body)
		},
		// Stremio's Cinemeta mirrors IMDb data
		func(ctx context.Context) (*Score, error) {
			typ := "movie"
			if kind == "tv" {
				typ = "series"
			}
			body, _, err := c.fetch(ctx, cinemeta+"/meta/"+typ+"/"+id+".json")
			if err != nil {
				return nil, err
			}
			return parseCinemeta(body)
		},
		// the title page itself
		func(ctx context.Context) (*Score, error) {
			body, _, err := c.fetch(ctx, imdbSite+"/title/"+id+"/")
			if err != nil {
				return nil, err
			}
			return parseLDRating(body)
		},
	)
	if s != nil {
		s.URL = page
	}
	return s
}

func parseIMDbJSONP(body string) (*Score, error) {
	i, j := strings.Index(body, "{"), strings.LastIndex(body, "}")
	if i < 0 || j < i {
		return nil, errNoScore
	}
	var v struct {
		Resource struct {
			Rating float64 `json:"rating"`
			Count  int     `json:"ratingCount"`
		} `json:"resource"`
	}
	if err := json.Unmarshal([]byte(body[i:j+1]), &v); err != nil {
		return nil, err
	}
	return &Score{Value: v.Resource.Rating, Votes: v.Resource.Count, From: "imdb"}, nil
}

func parseCinemeta(body string) (*Score, error) {
	var v struct {
		Meta struct {
			Rating string `json:"imdbRating"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	f, err := strconv.ParseFloat(v.Meta.Rating, 64)
	if err != nil {
		return nil, errNoScore
	}
	return &Score{Value: f, From: "cinemeta"}, nil
}

var (
	ldRating = regexp.MustCompile(`"aggregateRating"\s*:\s*\{[^}]*\}`)
	ldValue  = regexp.MustCompile(`"ratingValue"\s*:\s*"?([0-9.]+)`)
	ldCount  = regexp.MustCompile(`"ratingCount"\s*:\s*"?([0-9]+)`)
	lbMeta   = regexp.MustCompile(`name="twitter:data2"\s+content="([0-9.]+) out of 5"`)
)

// parseLDRating reads schema.org aggregateRating from a page's JSON-LD.
func parseLDRating(body string) (*Score, error) {
	block := ldRating.FindString(body)
	if block == "" {
		return nil, errNoScore
	}
	m := ldValue.FindStringSubmatch(block)
	if m == nil {
		return nil, errNoScore
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	s := &Score{Value: f, From: "page"}
	if m := ldCount.FindStringSubmatch(block); m != nil {
		s.Votes, _ = strconv.Atoi(m[1])
	}
	return s, nil
}

// ---------- Letterboxd ----------

// letterboxd opens letterboxd.com/tmdb/<id>/, which redirects to the film's
// page, and reads its average from the page.
func (c *Client) letterboxd(ctx context.Context, tmdbID int64) *Score {
	body, final, err := c.fetch(ctx, fmt.Sprintf("%s/tmdb/%d/", letterboxd, tmdbID))
	if err != nil {
		return nil
	}
	s, err := parseLDRating(body)
	if err != nil {
		m := lbMeta.FindStringSubmatch(body)
		if m == nil {
			return nil
		}
		f, _ := strconv.ParseFloat(m[1], 64)
		s = &Score{Value: f}
	}
	s.From, s.URL = "letterboxd", final
	if s.Value <= 0 || s.Value > 5 {
		return nil
	}
	return s
}
