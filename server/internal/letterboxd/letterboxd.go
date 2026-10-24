// Package letterboxd reads what a Letterboxd member exports (the data ZIP
// from Settings → Data, or one of its CSV files) and their public RSS feed,
// and writes CSV files that Letterboxd's own importer accepts.
//
// Letterboxd has no public API for personal apps, so there is no two-way
// sync: the export and the RSS feed go one way in, CSV files go one way out.
package letterboxd

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Entry is one film as Letterboxd knows it, with what the member did with it.
type Entry struct {
	Name    string
	Year    int
	Date    int64 // when it was logged, liked or added (unix), 0 if unknown
	Rating  int   // 1–10 (half stars × 2), 0 if not rated
	Watched bool
	Liked   bool
	Later   bool
	// From the RSS feed only, which names the TMDB title directly.
	Kind string // movie | tv
	TMDB int64
}

func (e Entry) key() string { return strings.ToLower(e.Name) + "|" + strconv.Itoa(e.Year) }

// ErrUnknownFile is returned for a CSV that is not one of the export's files
// and no list to put it in was given.
var ErrUnknownFile = errors.New("не похоже на файл экспорта Letterboxd")

// ParseFile reads an uploaded export: the ZIP, or a single CSV from it. A CSV
// whose name does not tell its list (a Letterboxd list export, say) goes to
// fallback ("liked", "later" or "watched"); with no fallback it is refused.
func ParseFile(name string, data []byte, fallback string) ([]Entry, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return parseZip(data)
	}
	list := listFor(name)
	if list == "" {
		list = fallback
	}
	if list == "" {
		return nil, ErrUnknownFile
	}
	m := newMerger()
	if err := m.addCSV(bytes.NewReader(data), list); err != nil {
		return nil, err
	}
	return m.out, nil
}

// listFor maps a file of the export to the list it fills; "" means skip.
func listFor(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, `\`, "/"))
	dir, base := path.Split(name)
	dir = strings.Trim(dir, "/")
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		dir = dir[i+1:]
	}
	switch {
	case dir == "likes" && base == "films.csv":
		return "liked"
	case strings.HasPrefix(base, "watchlist"):
		return "later"
	case dir == "lists" || dir == "deleted" || dir == "orphaned" || dir == "likes":
		return ""
	case base == "watched.csv" || base == "ratings.csv" || base == "diary.csv" || base == "reviews.csv":
		return "watched"
	}
	return ""
}

func parseZip(data []byte) ([]Entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("архив не читается: %w", err)
	}
	m := newMerger()
	found := false
	for _, f := range zr.File {
		list := listFor(f.Name)
		if list == "" || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		err = m.addCSV(io.LimitReader(rc, 64<<20), list)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		found = true
	}
	if !found {
		return nil, ErrUnknownFile
	}
	return m.out, nil
}

type merger struct {
	out []Entry
	idx map[string]int
}

func newMerger() *merger { return &merger{idx: map[string]int{}} }

func (m *merger) add(e Entry) {
	i, ok := m.idx[e.key()]
	if !ok {
		m.idx[e.key()] = len(m.out)
		m.out = append(m.out, e)
		return
	}
	x := &m.out[i]
	x.Watched = x.Watched || e.Watched
	x.Liked = x.Liked || e.Liked
	x.Later = x.Later || e.Later
	if e.Rating > 0 && (x.Rating == 0 || e.Date >= x.Date) {
		x.Rating = e.Rating // the latest diary rating wins
	}
	if e.Date > x.Date {
		x.Date = e.Date
	}
	if x.TMDB == 0 {
		x.TMDB, x.Kind = e.TMDB, e.Kind
	}
}

// addCSV reads one export file. Letterboxd list exports start with a few
// lines about the list itself, so the header is the first row naming both
// Name and Year.
func (m *merger) addCSV(r io.Reader, list string) error {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	col := map[string]int{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(col) == 0 {
			h := map[string]int{}
			for i, c := range rec {
				h[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, "\uFEFF")))] = i
			}
			_, okName := h["name"]
			_, okYear := h["year"]
			if okName && okYear {
				col = h
			}
			continue
		}
		get := func(k string) string {
			if i, ok := col[k]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		e := Entry{Name: get("name")}
		if e.Name == "" {
			continue
		}
		e.Year, _ = strconv.Atoi(get("year"))
		e.Rating = parseRating(get("rating"))
		e.Date = parseDate(get("watched date"))
		if e.Date == 0 {
			e.Date = parseDate(get("date"))
		}
		switch list {
		case "liked":
			e.Liked = true
		case "later":
			e.Later = true
		default:
			e.Watched = true
		}
		m.add(e)
	}
	if len(col) == 0 {
		return ErrUnknownFile
	}
	return nil
}

// parseRating turns Letterboxd stars ("3.5") into 1–10.
func parseRating(s string) int {
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return min(10, int(f*2+0.5))
}

func parseDate(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{"2006-01-02", time.RFC1123Z, time.RFC1123} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// ---------- RSS ----------

type rssItem struct {
	PubDate   string `xml:"pubDate"`
	FilmTitle string `xml:"filmTitle"`
	FilmYear  int    `xml:"filmYear"`
	Watched   string `xml:"watchedDate"`
	Rating    string `xml:"memberRating"`
	Like      string `xml:"memberLike"`
	TMDBMovie int64  `xml:"movieId"`
	TMDBShow  int64  `xml:"tvId"`
}

// ParseRSS reads a member's feed (letterboxd.com/<user>/rss/): the latest
// diary entries, each naming its TMDB title. Reviews and lists in the feed
// are skipped.
func ParseRSS(r io.Reader) ([]Entry, error) {
	var feed struct {
		Items []rssItem `xml:"channel>item"`
	}
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("RSS не читается: %w", err)
	}
	m := newMerger()
	for _, it := range feed.Items {
		if it.FilmTitle == "" || it.TMDBMovie == 0 && it.TMDBShow == 0 {
			continue
		}
		e := Entry{Name: it.FilmTitle, Year: it.FilmYear, Watched: true, Kind: "movie", TMDB: it.TMDBMovie,
			Rating: parseRating(it.Rating), Liked: strings.EqualFold(it.Like, "yes")}
		if e.TMDB == 0 {
			e.Kind, e.TMDB = "tv", it.TMDBShow
		}
		if e.Date = parseDate(it.Watched); e.Date == 0 {
			e.Date = parseDate(it.PubDate)
		}
		m.add(e)
	}
	return m.out, nil
}

// ---------- export in Letterboxd's import format ----------

// Film is one row of a CSV for letterboxd.com/import (or the watchlist
// importer). The TMDB id lets Letterboxd match the film exactly.
type Film struct {
	TMDB    int64
	Title   string
	Year    int
	Rating  int   // 1–10, 0 if not rated
	Watched int64 // unix, 0 if unknown
	Liked   bool
}

// WriteCSV writes films in the format Letterboxd's importer documents. It has
// no column for likes, so liked films carry the tag "liked".
func WriteCSV(w io.Writer, films []Film) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"tmdbID", "Title", "Year", "Rating", "WatchedDate", "Tags"})
	for _, f := range films {
		rating, date, tags, year := "", "", "", ""
		if f.Rating > 0 {
			rating = strconv.FormatFloat(float64(f.Rating)/2, 'f', -1, 64)
		}
		if f.Watched > 0 {
			date = time.Unix(f.Watched, 0).Format("2006-01-02")
		}
		if f.Liked {
			tags = "liked"
		}
		if f.Year > 0 {
			year = strconv.Itoa(f.Year)
		}
		_ = cw.Write([]string{strconv.FormatInt(f.TMDB, 10), f.Title, year, rating, date, tags})
	}
	cw.Flush()
	return cw.Error()
}
