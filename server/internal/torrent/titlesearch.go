package torrent

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// TitleQuery describes a catalog title to look up on the user's indexers.
type TitleQuery struct {
	Names  []string // localized and original titles
	Year   int
	Show   bool
	Season int // 0 = any
}

// SearchTitle searches every name of a title (trackers name releases in
// either language), merges duplicates and drops releases that do not mention
// the title. loose is true when nothing matched and the unfiltered results are
// returned instead.
func (e *Engine) SearchTitle(ctx context.Context, tq TitleQuery) (res []Result, errs []string, loose bool) {
	queries := titleQueries(tq)
	var mu sync.Mutex
	var all []Result
	seenErr := map[string]bool{}
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			r, es := e.Search(ctx, q)
			mu.Lock()
			defer mu.Unlock()
			all = append(all, r...)
			for _, er := range es {
				if !seenErr[er] {
					seenErr[er] = true
					errs = append(errs, er)
				}
			}
		}(q)
	}
	wg.Wait()
	all = dedupe(all)
	prefs := e.Prefs()
	matched := make([]Result, 0, len(all))
	for _, r := range all {
		if ok, bonus := relevant(r.Title, tq); ok {
			r.Score = score(r, prefs) + bonus
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 && len(all) > 0 {
		loose = true
		matched = all
		for i := range matched {
			matched[i].Score = score(matched[i], prefs)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].Score > matched[j].Score })
	for i := range matched {
		matched[i].Best = false
	}
	if len(matched) > 0 && !loose && matched[0].Seeds >= prefs.MinSeeds {
		matched[0].Best = true
	}
	return matched, errs, loose
}

func titleQueries(tq TitleQuery) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range tq.Names {
		n = strings.TrimSpace(n)
		if n == "" || seen[normTitle(n)] {
			continue
		}
		seen[normTitle(n)] = true
		q := n
		if !tq.Show && tq.Year > 0 {
			q += " " + strconv.Itoa(tq.Year)
		}
		out = append(out, q)
	}
	return out
}

var reBtih = regexp.MustCompile(`(?i)xt=urn:btih:([a-z0-9]+)`)

// dedupe drops the same release found by two queries or two indexers.
func dedupe(list []Result) []Result {
	seen := map[string]bool{}
	out := list[:0]
	for _, r := range list {
		key := r.Link
		if m := reBtih.FindStringSubmatch(r.Link + " " + r.Magnet); m != nil {
			key = strings.ToLower(m[1])
		} else if r.Size > 0 {
			key = normTitle(r.Title) + "/" + strconv.FormatInt(r.Size, 10)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

var (
	reYear   = regexp.MustCompile(`(?:^|\D)((?:19|20)\d\d)(?:\D|$)`)
	reSeason = regexp.MustCompile(`(?i)(?:\bs(\d{1,2})(?:e\d|\b|-)|сезон[:\s]*(\d{1,2})|(\d{1,2})\s*сезон|season\s*(\d{1,2}))`)
)

// relevant tells whether a release title is about the queried title and how
// much to boost it (year and season matches).
func relevant(title string, tq TitleQuery) (bool, float64) {
	nt := normTitle(title)
	hit := false
	for _, n := range tq.Names {
		if n := normTitle(n); n != "" && strings.Contains(nt, n) {
			hit = true
			break
		}
	}
	if !hit {
		return false, 0
	}
	bonus := 0.0
	if tq.Year > 0 {
		years := reYear.FindAllStringSubmatch(title, -1)
		near := false
		for _, y := range years {
			v, _ := strconv.Atoi(y[1])
			if d := v - tq.Year; d >= -1 && d <= 1 || tq.Show && v >= tq.Year {
				near = true
			}
		}
		switch {
		case near:
			bonus += 10
		case len(years) > 0 && !tq.Show:
			return false, 0 // a remake or a namesake from another year
		}
	}
	if tq.Show && tq.Season > 0 {
		if seasonMatches(title, tq.Season) {
			bonus += 15
		} else {
			bonus -= 10
		}
	}
	return true, bonus
}

var reRange = regexp.MustCompile(`(?i)(?:s|сезон[ыа]?[:\s]*)(\d{1,2})\s*-\s*(?:s)?(\d{1,2})`)

func seasonMatches(title string, season int) bool {
	for _, m := range reRange.FindAllStringSubmatch(title, -1) {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if season >= a && season <= b {
			return true
		}
	}
	for _, m := range reSeason.FindAllStringSubmatch(title, -1) {
		for _, g := range m[1:] {
			if v, err := strconv.Atoi(g); err == nil && v == season {
				return true
			}
		}
	}
	return false
}

// normTitle lowercases and strips everything but letters and digits, so
// "Дюна: Часть вторая" matches "Дюна. Часть вторая / Dune: Part Two".
func normTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == 'ё' {
			r = 'е'
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 'а' && r <= 'я' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
