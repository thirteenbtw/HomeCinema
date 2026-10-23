// Package parse extracts title, year, season and episode from media file names,
// in the spirit of guessit (FR-2).
package parse

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Result struct {
	Kind    string // movie | episode
	Title   string
	Year    int
	Season  int
	Episode int
	// Episode2 is set for multi-episode files like S01E01E02.
	Episode2 int
	Quality  string
}

var VideoExts = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".webm": true, ".ts": true, ".m2ts": true, ".wmv": true, ".mpg": true, ".mpeg": true}
var SubExts = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true}

func IsVideo(path string) bool { return VideoExts[strings.ToLower(filepath.Ext(path))] }

var (
	reSxxExx   = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]?E(\d{1,3})(?:[ ._-]?E?(\d{1,3}))?\b`)
	reNxNN     = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	reSeasonRu = regexp.MustCompile(`(?i)(\d{1,2})\s*сезон|сезон\s*(\d{1,2})|season\s*(\d{1,2})`)
	reEpRu     = regexp.MustCompile(`(?i)(\d{1,3})\s*сери[яи]|сери[яи]\s*(\d{1,3})|episode\s*(\d{1,3})|\bep?\.?\s?(\d{1,3})\b`)
	reYear     = regexp.MustCompile(`(?:^|[\s.\-_(\[])((?:19|20)\d{2})(?:$|[\s.\-_)\]])`)
	reSample   = regexp.MustCompile(`(?i)(^|[ ._-])sample([ ._-]|$)`)
	// Tokens after which the rest of a release name is technical noise.
	reNoise     = regexp.MustCompile(`(?i)[\s.\-_\[(](2160p|1080p|1080i|720p|576p|480p|4k|uhd|hdr10\+?|hdr|dv|dolby[\s.]?vision|bluray|blu-ray|bdrip|brrip|bdremux|remux|web[\s.-]?dl|webrip|web|hdtv|hdrip|dvdrip|dvd5|dvd9|x264|x265|h\.?264|h\.?265|hevc|avc|xvid|divx|aac|ac3|dts|ddp?5\.1|atmos|10bit|proper|repack|extended|unrated|imax|rus|eng|dub|mvo|avo|line|itunes|amzn|nf|hmax|kp|lostfilm|newstudio|baibako)(?:$|[\s.\-_\])])`)
	reBrackets  = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	reSeasonTag = regexp.MustCompile(`(?i)(?:^|[ ._-])s(\d{1,2})(?:$|[ ._-])`)
	reQuality   = regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|4k)`)
)

// IsSample reports files like "movie-sample.mkv" that should not be imported.
func IsSample(path string) bool {
	return reSample.MatchString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

// Parse looks at the file name first and falls back to parent folders for the
// show title or movie year when the file name alone is not enough.
func Parse(path string) Result {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	file := parts[len(parts)-1]
	name := strings.ReplaceAll(strings.TrimSuffix(file, filepath.Ext(file)), "_", " ")
	var parents []string
	for i := len(parts) - 2; i >= 0 && len(parents) < 3; i-- {
		if parts[i] != "" {
			parents = append(parents, parts[i])
		}
	}

	r := Result{Kind: "movie"}
	if q := reQuality.FindString(name); q != "" {
		r.Quality = strings.ToLower(q)
	}

	// Episode markers in the file name.
	if m := reSxxExx.FindStringSubmatchIndex(name); m != nil {
		r.Kind = "episode"
		r.Season = atoi(name[m[2]:m[3]])
		r.Episode = atoi(name[m[4]:m[5]])
		if m[6] > 0 {
			r.Episode2 = atoi(name[m[6]:m[7]])
		}
		r.Title = cleanTitle(name[:m[0]])
	} else if m := reNxNN.FindStringSubmatchIndex(name); m != nil && !looksLikeResolution(name, m) {
		r.Kind = "episode"
		r.Season = atoi(name[m[2]:m[3]])
		r.Episode = atoi(name[m[4]:m[5]])
		r.Title = cleanTitle(name[:m[0]])
	} else if seasonFromParents(parents) > 0 {
		// "Сериал/Сезон 2/03 серия.mkv" or "Show/Season 1/Episode 05.mkv"
		if ep := episodeFromName(name); ep > 0 {
			r.Kind = "episode"
			r.Season = seasonFromParents(parents)
			r.Episode = ep
		}
	}

	if r.Kind == "episode" {
		if r.Title == "" {
			r.Title = showTitleFromParents(parents)
		}
		r.Title, r.Year = splitYear(r.Title)
		if r.Year == 0 {
			for _, p := range parents {
				if _, y := splitYear(cleanTitle(p)); y > 0 {
					r.Year = y
					break
				}
			}
		}
		return r
	}

	r.Title, r.Year = movieTitle(name)
	// "Movie (2010)/movie.mkv" or a generic file name inside a properly named folder.
	if (r.Year == 0 || len(r.Title) < 2 || isGeneric(r.Title)) && len(parents) > 0 {
		if t, y := movieTitle(parents[0]); t != "" && (y > 0 || isGeneric(r.Title)) {
			r.Title, r.Year = t, y
		}
	}
	return r
}

func movieTitle(name string) (string, int) {
	name = reBrackets.ReplaceAllString(name, " ")
	// Year: take the last plausible year so titles like "2001 A Space Odyssey 1968" work.
	year := 0
	cut := -1
	for _, m := range reYear.FindAllStringSubmatchIndex(name, -1) {
		if m[2] == 0 { // a year at the very start is part of the title
			continue
		}
		year = atoi(name[m[2]:m[3]])
		cut = m[0]
	}
	if cut >= 0 {
		name = name[:cut]
	}
	return cleanTitle(name), year
}

func splitYear(title string) (string, int) {
	if m := reYear.FindStringSubmatchIndex(title); m != nil && m[2] > 0 {
		return strings.TrimSpace(strings.Trim(title[:m[0]], " -")), atoi(title[m[2]:m[3]])
	}
	return title, 0
}

func cleanTitle(s string) string {
	s = reBrackets.ReplaceAllString(s, " ")
	if loc := reNoise.FindStringIndex(" " + s + " "); loc != nil {
		cut := loc[0] - 1
		if cut < 0 {
			cut = 0
		}
		if cut <= len(s) {
			s = s[:cut]
		}
	}
	// Dots and underscores are separators unless the title is an acronym like "S.W.A.T."
	if !regexp.MustCompile(`^([A-Za-z]\.){2,}`).MatchString(s) {
		s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	} else {
		s = strings.ReplaceAll(s, "_", " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " -–—:(")
	return smartCase(s)
}

// smartCase title-cases names that are entirely lower-case.
func smartCase(s string) string {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return s
		}
	}
	words := strings.Fields(s)
	for i, w := range words {
		rs := []rune(w)
		rs[0] = unicode.ToUpper(rs[0])
		words[i] = string(rs)
	}
	return strings.Join(words, " ")
}

func seasonFromParents(parents []string) int {
	for _, p := range parents {
		if m := reSeasonRu.FindStringSubmatch(p); m != nil {
			for _, g := range m[1:] {
				if g != "" {
					return atoi(g)
				}
			}
		}
		if m := reSeasonTag.FindStringSubmatch(p); m != nil {
			return atoi(m[1])
		}
	}
	return 0
}

func showTitleFromParents(parents []string) string {
	for _, p := range parents {
		if seasonFromParents([]string{p}) > 0 {
			// "Show.Name.S02.1080p" style season folders carry the show name too.
			if m := regexp.MustCompile(`(?i)^(.*?)[ ._-]+(s\d{1,2}|season|сезон)`).FindStringSubmatch(p); m != nil && strings.TrimSpace(m[1]) != "" {
				return cleanTitle(m[1])
			}
			continue
		}
		return cleanTitle(p)
	}
	return ""
}

func episodeFromName(name string) int {
	if m := reEpRu.FindStringSubmatch(name); m != nil {
		for _, g := range m[1:] {
			if g != "" {
				return atoi(g)
			}
		}
	}
	if m := regexp.MustCompile(`^\s*(\d{1,3})\b`).FindStringSubmatch(name); m != nil {
		return atoi(m[1])
	}
	return 0
}

func looksLikeResolution(name string, m []int) bool {
	s := strings.ToLower(name[m[0]:m[1]])
	return s == "1920x1080" || s == "1280x720" || strings.HasPrefix(s, "19x") || m[3]-m[2] > 2
}

var genericNames = map[string]bool{"movie": true, "film": true, "фильм": true, "video": true, "main": true, "feature": true}

func isGeneric(t string) bool { return genericNames[strings.ToLower(t)] }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
