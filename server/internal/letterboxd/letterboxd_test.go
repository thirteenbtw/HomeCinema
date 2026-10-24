package letterboxd

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestParseZip(t *testing.T) {
	data := zipOf(t, map[string]string{
		"letterboxd-user-2026-10-24/watched.csv":     "Date,Name,Year,Letterboxd URI\n2024-03-01,Dune: Part Two,2024,https://boxd.it/x\n2023-01-02,\"Parasite\",2019,https://boxd.it/y\n",
		"letterboxd-user-2026-10-24/ratings.csv":     "Date,Name,Year,Letterboxd URI,Rating\n2024-03-01,Dune: Part Two,2024,https://boxd.it/x,4.5\n",
		"letterboxd-user-2026-10-24/watchlist.csv":   "Date,Name,Year,Letterboxd URI\n2025-05-05,Stalker,1979,https://boxd.it/z\n",
		"letterboxd-user-2026-10-24/likes/films.csv": "Date,Name,Year,Letterboxd URI\n2023-01-02,Parasite,2019,https://boxd.it/y\n",
		"letterboxd-user-2026-10-24/lists/best.csv":  "Letterboxd list export v7\nDate,Name,Tags,URL,Description\n\nPosition,Name,Year,URL,Description\n1,Ignored,2000,u,\n",
	})
	got, err := ParseFile("export.zip", data, "")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Entry{}
	for _, e := range got {
		by[e.Name] = e
	}
	if len(by) != 3 {
		t.Fatalf("%+v", got)
	}
	if d := by["Dune: Part Two"]; !d.Watched || d.Rating != 9 || d.Year != 2024 || d.Liked {
		t.Fatalf("dune %+v", d)
	}
	if p := by["Parasite"]; !p.Watched || !p.Liked {
		t.Fatalf("parasite %+v", p)
	}
	if s := by["Stalker"]; !s.Later || s.Watched || s.Date == 0 {
		t.Fatalf("stalker %+v", s)
	}
}

func TestParseListCSV(t *testing.T) {
	list := "\uFEFFLetterboxd list export v7\nDate,Name,Tags,URL,Description\n2024-01-01,Best,,https://letterboxd.com/u/list/best/,\n\nPosition,Name,Year,URL,Description\n1,Solaris,1972,https://boxd.it/a,\n"
	if _, err := ParseFile("best.csv", []byte(list), ""); err != ErrUnknownFile {
		t.Fatalf("err %v", err)
	}
	got, err := ParseFile("best.csv", []byte(list), "later")
	if err != nil || len(got) != 1 || got[0].Name != "Solaris" || got[0].Year != 1972 || !got[0].Later {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestParseRSS(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:letterboxd="https://letterboxd.com" xmlns:tmdb="https://themoviedb.org" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title>Letterboxd - user</title>
<item><title>Dune: Part Two, 2024 - ★★★★½</title><pubDate>Fri, 1 Mar 2024 20:00:00 +1300</pubDate>
 <letterboxd:watchedDate>2024-03-01</letterboxd:watchedDate><letterboxd:rewatch>No</letterboxd:rewatch>
 <letterboxd:filmTitle>Dune: Part Two</letterboxd:filmTitle><letterboxd:filmYear>2024</letterboxd:filmYear>
 <letterboxd:memberRating>4.5</letterboxd:memberRating><letterboxd:memberLike>Yes</letterboxd:memberLike>
 <tmdb:movieId>693134</tmdb:movieId></item>
<item><title>A list</title><pubDate>Fri, 1 Mar 2024 20:00:00 +1300</pubDate></item>
</channel></rss>`
	got, err := ParseRSS(strings.NewReader(feed))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	e := got[0]
	if e.TMDB != 693134 || e.Kind != "movie" || e.Rating != 9 || !e.Liked || !e.Watched || e.Date == 0 {
		t.Fatalf("%+v", e)
	}
}

func TestWriteCSV(t *testing.T) {
	var b bytes.Buffer
	if err := WriteCSV(&b, []Film{{TMDB: 693134, Title: "Dune, Part Two", Year: 2024, Rating: 9, Watched: 1709290000, Liked: true}, {TMDB: 1, Title: "X"}}); err != nil {
		t.Fatal(err)
	}
	want := "tmdbID,Title,Year,Rating,WatchedDate,Tags\n693134,\"Dune, Part Two\",2024,4.5,2024-03-01,liked\n1,X,,,,\n"
	if b.String() != want {
		t.Fatalf("%q", b.String())
	}
}

func TestUser(t *testing.T) {
	for in, want := range map[string]string{"dave": "dave", " @dave ": "dave", "https://letterboxd.com/dave/films/": "dave", "letterboxd.com/dave": "dave", "x y": "", "": ""} {
		if got := User(in); got != want {
			t.Errorf("User(%q) = %q", in, got)
		}
	}
}
