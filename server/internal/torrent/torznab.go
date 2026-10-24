package torrent

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Indexer is a search source: a Torznab endpoint the user adds (Jackett,
// Prowlarr or any other Torznab-compatible service), or a public JacRed
// parser, which new installs start with.
type Indexer struct {
	Name   string `json:"name"`
	URL    string `json:"url"` // e.g. http://127.0.0.1:9117/api/v2.0/indexers/all/results/torznab/
	APIKey string `json:"apiKey"`
	On     bool   `json:"on"`
	Kind   string `json:"kind,omitempty"` // "" = Torznab, KindJacRed
}

type SearchPrefs struct {
	Quality   string `json:"quality"`   // 2160p | 1080p | 720p | ""
	Language  string `json:"language"`  // rus | eng | ""
	MaxSizeGB int    `json:"maxSizeGB"` // 0 = any
	MinSeeds  int    `json:"minSeeds"`
}

type Result struct {
	Indexer string  `json:"indexer"`
	Title   string  `json:"title"`
	Link    string  `json:"link"`             // .torrent URL, or the magnet when there is none
	Magnet  string  `json:"magnet,omitempty"` // fallback when the .torrent cannot be fetched
	Size    int64   `json:"size"`
	Seeds   int     `json:"seeds"`
	Peers   int     `json:"peers"`
	Quality string  `json:"quality"`
	Lang    string  `json:"lang"`
	Date    string  `json:"date"`
	Score   float64 `json:"score"`
	Best    bool    `json:"best"`
}

type rss struct {
	Channel struct {
		Items []struct {
			Title     string `xml:"title"`
			Link      string `xml:"link"`
			Size      int64  `xml:"size"`
			PubDate   string `xml:"pubDate"`
			Enclosure struct {
				URL    string `xml:"url,attr"`
				Length int64  `xml:"length,attr"`
			} `xml:"enclosure"`
			Attrs []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"attr"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (e *Engine) Indexers() []Indexer {
	var out []Indexer
	_ = json.Unmarshal([]byte(e.Store.Setting("indexers", "[]")), &out)
	if e.Store.Setting("jacredAdded", "") == "" {
		// once, for new and existing installs; removing it afterwards sticks.
		// JacRed sites added by hand as Torznab are covered by it.
		kept := defaultIndexers()
		for _, in := range out {
			if !isJacRedSite(in) {
				kept = append(kept, in)
			}
		}
		out = kept
		if e.SetIndexers(out) == nil {
			_ = e.Store.SetSetting("jacredAdded", "1")
		}
	}
	if out == nil {
		out = []Indexer{}
	}
	return out
}

func (e *Engine) SetIndexers(list []Indexer) error {
	b, _ := json.Marshal(list)
	return e.Store.SetSetting("indexers", string(b))
}

func (e *Engine) Prefs() SearchPrefs {
	p := SearchPrefs{Quality: "1080p", Language: "rus", MinSeeds: 3}
	_ = json.Unmarshal([]byte(e.Store.Setting("searchPrefs", "{}")), &p)
	return p
}

// Search queries all enabled indexers in parallel and ranks the results by the
// preference profile; the top one is marked Best for one-click "watch now".
func (e *Engine) Search(ctx context.Context, query string) ([]Result, []string) {
	idx := e.Indexers()
	prefs := e.Prefs()
	var mu sync.Mutex
	var all []Result
	var errs []string
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 25 * time.Second}
	for _, in := range idx {
		if !in.On {
			continue
		}
		wg.Add(1)
		go func(in Indexer) {
			defer wg.Done()
			var res []Result
			var err error
			if in.Kind == KindJacRed || isJacRedSite(in) {
				res, err = queryJacRed(ctx, client, in, query)
			} else {
				res, err = queryTorznab(ctx, client, in, query)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, in.Name+": "+err.Error())
				return
			}
			all = append(all, res...)
		}(in)
	}
	wg.Wait()
	for i := range all {
		all[i].Score = score(all[i], prefs)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > 0 && all[0].Seeds >= prefs.MinSeeds {
		all[0].Best = true
	}
	if all == nil {
		all = []Result{}
	}
	return all, errs
}

func queryTorznab(ctx context.Context, client *http.Client, in Indexer, query string) ([]Result, error) {
	u, err := url.Parse(in.URL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("t", "search")
	q.Set("q", query)
	if in.APIKey != "" {
		q.Set("apikey", in.APIKey)
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var feed rss
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("не Torznab-ответ: %w", err)
	}
	var out []Result
	for _, it := range feed.Channel.Items {
		r := Result{Indexer: in.Name, Title: it.Title, Link: it.Link, Size: it.Size, Date: it.PubDate}
		if r.Size == 0 {
			r.Size = it.Enclosure.Length
		}
		if it.Enclosure.URL != "" && r.Link == "" {
			r.Link = it.Enclosure.URL
		}
		for _, a := range it.Attrs {
			switch a.Name {
			case "seeders":
				r.Seeds, _ = strconv.Atoi(a.Value)
			case "peers":
				r.Peers, _ = strconv.Atoi(a.Value)
			case "magneturl":
				r.Magnet = a.Value
			case "infohash":
				if r.Magnet == "" && len(a.Value) == 40 {
					r.Magnet = "magnet:?xt=urn:btih:" + strings.ToLower(a.Value) + "&dn=" + url.QueryEscape(it.Title)
				}
			case "size":
				if r.Size == 0 {
					r.Size, _ = strconv.ParseInt(a.Value, 10, 64)
				}
			}
		}
		// A .torrent carries the file list, so the download starts at once; a
		// magnet first has to fetch it from peers. Keep both.
		if strings.HasPrefix(r.Link, "magnet:") {
			if r.Magnet == "" {
				r.Magnet = r.Link
			}
		}
		if r.Link == "" || strings.HasPrefix(r.Link, "magnet:") {
			r.Link, r.Magnet = r.Magnet, ""
		}
		r.Quality = detectQuality(r.Title)
		r.Lang = detectLang(r.Title)
		out = append(out, r)
	}
	return out, nil
}

var (
	reQ2160 = regexp.MustCompile(`(?i)2160p|4k|uhd`)
	reQ1080 = regexp.MustCompile(`(?i)1080[pi]`)
	reQ720  = regexp.MustCompile(`(?i)720p`)
	reRus   = regexp.MustCompile(`(?i)\b(rus|дубляж|dub|mvo|avo|лицензия|itunes|рус)`)
)

func detectQuality(t string) string {
	switch {
	case reQ2160.MatchString(t):
		return "2160p"
	case reQ1080.MatchString(t):
		return "1080p"
	case reQ720.MatchString(t):
		return "720p"
	}
	return ""
}

func detectLang(t string) string {
	if reRus.MatchString(t) || strings.ContainsAny(t, "абвгдеёжзийклмнопрстуфхцчшщыэюя") {
		return "rus"
	}
	return "eng"
}

func score(r Result, p SearchPrefs) float64 {
	s := 0.0
	if r.Seeds < p.MinSeeds {
		s -= 50
	}
	s += min(float64(r.Seeds), 200) / 10 // up to +20 for a healthy swarm
	if p.Quality != "" && r.Quality == p.Quality {
		s += 15
	} else if r.Quality != "" {
		s += 5
	}
	if p.Language != "" && r.Lang == p.Language {
		s += 12
	}
	if p.MaxSizeGB > 0 && r.Size > int64(p.MaxSizeGB)<<30 {
		s -= 30
	}
	if strings.HasPrefix(r.Link, "http") {
		s += 1 // a .torrent file skips the metadata wait
	}
	return s
}

// FetchTorrent downloads a .torrent link returned by an indexer.
func FetchTorrent(ctx context.Context, link string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme == "magnet" {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		// Some indexers redirect to a magnet URI, which net/http reports as an error.
		if ue, ok := err.(*url.Error); ok && strings.HasPrefix(ue.URL, "magnet:") {
			return nil, ue.URL, nil
		}
		return nil, "", err
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "magnet:") {
		return nil, loc, nil
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b := make([]byte, 0, 64<<10)
	buf := make([]byte, 32<<10)
	for len(b) < 10<<20 {
		n, err := resp.Body.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			break
		}
	}
	return b, "", nil
}
