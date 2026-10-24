package ratings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/static-content/documents/v1/title/tt1160419/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`imdb.rating.run({"@meta":{},"resource":{"@type":"imdb.api.title.ratings","id":"/title/tt1160419/","rating":8.0,"ratingCount":1000000}})`))
	})
	mux.HandleFunc("/meta/movie/tt1160419.json", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "slow", 500) })
	mux.HandleFunc("/title/tt1160419/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "waf", 202) })
	mux.HandleFunc("/tmdb/438631/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/film/dune-2021/", 302) })
	mux.HandleFunc("/film/dune-2021/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<meta name="twitter:data2" content="3.94 out of 5" /><script type="application/ld+json">
/* <![CDATA[ */ {"@type":"Movie","aggregateRating":{"bestRating":5,"reviewCount":400000,"@type":"aggregateRating","ratingValue":3.94,"ratingCount":1500000,"worstRating":0.5}} /* ]]> */</script>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	imdbJSONP, cinemeta, imdbSite, letterboxd = srv.URL, srv.URL, srv.URL, srv.URL

	r := New().Get(context.Background(), "movie", 438631, "tt1160419")
	if r.IMDb == nil || r.IMDb.Value != 8.0 || r.IMDb.Votes != 1000000 || r.IMDb.URL != "https://www.imdb.com/title/tt1160419/" {
		t.Fatalf("imdb %+v", r.IMDb)
	}
	if r.Letterboxd == nil || r.Letterboxd.Value != 3.94 || r.Letterboxd.Votes != 1500000 || r.Letterboxd.URL != srv.URL+"/film/dune-2021/" {
		t.Fatalf("letterboxd %+v", r.Letterboxd)
	}
	tv := New().Get(context.Background(), "tv", 1, "")
	if tv.IMDb != nil || tv.Letterboxd != nil {
		t.Fatalf("%+v", tv)
	}
}

func TestParsers(t *testing.T) {
	if s, err := parseCinemeta(`{"meta":{"imdbRating":"9.5"}}`); err != nil || s.Value != 9.5 {
		t.Fatal(s, err)
	}
	if s, err := parseLDRating(`<script type="application/ld+json">{"aggregateRating":{"@type":"AggregateRating","ratingCount":2245081,"bestRating":10,"worstRating":1,"ratingValue":9.5}}</script>`); err != nil || s.Value != 9.5 || s.Votes != 2245081 {
		t.Fatal(s, err)
	}
}
