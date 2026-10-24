package torrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJacRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2.0/indexers/all/results" || r.URL.Query().Get("Query") != "Дюна 2024" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"Results":[
			{"Tracker":"rutracker","Title":"Дюна: Часть вторая / Dune: Part Two (2024) WEB-DL 1080p | Дубляж","Link":"http://x/t.torrent","MagnetUri":"magnet:?xt=urn:btih:abc","Size":5000000000,"Seeders":120,"Peers":7,"PublishDate":"2024-04-01T00:00:00"},
			{"Tracker":"rutor","Title":"no link"}]}`))
	}))
	defer srv.Close()
	res, err := queryJacRed(context.Background(), http.DefaultClient, Indexer{Name: "JacRed", URL: srv.URL + "/", Kind: KindJacRed}, "Дюна 2024")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("%+v", res)
	}
	r := res[0]
	if r.Link != "magnet:?xt=urn:btih:abc" || r.Seeds != 120 || r.Quality != "1080p" || r.Lang != "rus" || r.Indexer != "JacRed · rutracker" {
		t.Fatalf("%+v", r)
	}
}

func TestJacRedDown(t *testing.T) {
	_, err := queryJacRed(context.Background(), http.DefaultClient, Indexer{Name: "JacRed", URL: "http://127.0.0.1:1", Kind: KindJacRed}, "x")
	if err == nil {
		t.Fatal("no error")
	}
}
