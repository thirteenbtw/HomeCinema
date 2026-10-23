package parse

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		path    string
		kind    string
		title   string
		year    int
		season  int
		episode int
	}{
		{`D:/Фильмы/Interstellar.2014.2160p.UHD.BluRay.x265.10bit.HDR.mkv`, "movie", "Interstellar", 2014, 0, 0},
		{`D:/Фильмы/The.Matrix.1999.1080p.BluRay.x264.mkv`, "movie", "The Matrix", 1999, 0, 0},
		{`D:/Фильмы/Blade Runner 2049 (2017) [1080p].mkv`, "movie", "Blade Runner 2049", 2017, 0, 0},
		{`D:/Фильмы/2001.A.Space.Odyssey.1968.720p.mkv`, "movie", "2001 A Space Odyssey", 1968, 0, 0},
		{`D:/Фильмы/Брат 2 (2000)/movie.mkv`, "movie", "Брат 2", 2000, 0, 0},
		{`D:/Фильмы/Тихий.горизонт.2025.WEB-DL.1080p.mkv`, "movie", "Тихий горизонт", 2025, 0, 0},
		{`D:/Фильмы/Дюна (2021)/Dune.2021.2160p.mkv`, "movie", "Dune", 2021, 0, 0},
		{`D:/Сериалы/Breaking.Bad.S01E02.720p.HDTV.x264.mkv`, "episode", "Breaking Bad", 0, 1, 2},
		{`D:/Сериалы/The Office (US)/Season 3/The.Office.US.S03E10.mkv`, "episode", "The Office US", 0, 3, 10},
		{`D:/Сериалы/Severance.2022.S02E05.1080p.WEB.mkv`, "episode", "Severance", 2022, 2, 5},
		{`D:/Сериалы/Friends/Season 1/1x05 - The One with the East German Laundry Detergent.avi`, "episode", "Friends", 0, 1, 5},
		{`D:/Сериалы/Станция Север/Сезон 2/05 серия.mkv`, "episode", "Станция Север", 0, 2, 5},
		{`D:/Сериалы/Dark.S01.1080p.NF.WEB-DL/Dark.S01E01.Secrets.mkv`, "episode", "Dark", 0, 1, 1},
		{`D:/Сериалы/Dark.S01.1080p.NF.WEB-DL/01.mkv`, "episode", "Dark", 0, 1, 1},
		{`D:/Сериалы/doctor_who_s10e01.mkv`, "episode", "Doctor Who", 0, 10, 1},
	}
	for _, c := range cases {
		r := Parse(c.path)
		if r.Kind != c.kind || r.Title != c.title || r.Year != c.year || r.Season != c.season || r.Episode != c.episode {
			t.Errorf("%s\n got  %+v\n want kind=%s title=%q year=%d s=%d e=%d", c.path, r, c.kind, c.title, c.year, c.season, c.episode)
		}
	}
}

func TestSample(t *testing.T) {
	if !IsSample("x/Movie.2020.sample.mkv") || IsSample("x/Sampler.2020.mkv") {
		t.Fatal("sample detection")
	}
}
