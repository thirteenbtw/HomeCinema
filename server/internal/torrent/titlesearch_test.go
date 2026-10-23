package torrent

import "testing"

func TestRelevant(t *testing.T) {
	dune := TitleQuery{Names: []string{"Дюна: Часть вторая", "Dune: Part Two"}, Year: 2024}
	cases := []struct {
		title string
		q     TitleQuery
		ok    bool
	}{
		{"Дюна: Часть вторая / Dune: Part Two (2024) WEB-DL 1080p | D", dune, true},
		{"Dune.Part.Two.2024.2160p.UHD.BluRay.x265", dune, true},
		{"Dune: Part Two 2024 1080p", dune, true},
		{"Дюна / Dune (2021) BDRip 1080p", dune, false},
		{"Dune Part Two (1984) fan edit", dune, false},
		{"Дюна: Часть вторая", dune, true}, // no year at all: keep
		{"Во все тяжкие / Breaking Bad / Сезон: 1-5 / Серии: 1-62 (2008-2013)", TitleQuery{Names: []string{"Во все тяжкие", "Breaking Bad"}, Year: 2008, Show: true}, true},
	}
	for _, c := range cases {
		if ok, _ := relevant(c.title, c.q); ok != c.ok {
			t.Errorf("relevant(%q) = %v, want %v", c.title, ok, c.ok)
		}
	}
}

func TestSeasonMatches(t *testing.T) {
	cases := []struct {
		title  string
		season int
		want   bool
	}{
		{"Breaking.Bad.S03.1080p.BluRay", 3, true},
		{"Breaking.Bad.S03E05.1080p", 3, true},
		{"Во все тяжкие / Сезон: 1-5 / Серии: 1-62", 4, true},
		{"Во все тяжкие / Сезон: 2 / Серии: 1-13", 4, false},
		{"Breaking Bad Season 2 Complete", 2, true},
		{"Breaking Bad S01-S05 Complete", 5, true},
		{"Breaking Bad 2 сезон", 2, true},
	}
	for _, c := range cases {
		if got := seasonMatches(c.title, c.season); got != c.want {
			t.Errorf("seasonMatches(%q, %d) = %v, want %v", c.title, c.season, got, c.want)
		}
	}
}

func TestDedupe(t *testing.T) {
	in := []Result{
		{Title: "A", Link: "magnet:?xt=urn:btih:ABC&dn=a"},
		{Title: "A copy", Link: "magnet:?xt=urn:btih:abc&dn=b"},
		{Title: "B", Link: "http://x/1", Size: 10},
		{Title: "B", Link: "http://y/1", Size: 10},
		{Title: "C", Link: "http://x/2", Size: 11},
	}
	if got := dedupe(in); len(got) != 3 {
		t.Fatalf("dedupe kept %d, want 3", len(got))
	}
}
