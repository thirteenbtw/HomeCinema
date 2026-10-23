package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestUsable(t *testing.T) {
	p := func(s ...string) []netip.Addr {
		var out []netip.Addr
		for _, x := range s {
			out = append(out, netip.MustParseAddr(x))
		}
		return out
	}
	if usable(p("127.0.0.1")) || usable(p("0.0.0.0")) || usable(nil) || usable(p("10.0.0.1")) {
		t.Fatal("blocked answer counted as usable")
	}
	if !usable(p("127.0.0.1", "13.224.1.2")) {
		t.Fatal("public address rejected")
	}
}

func TestQueryDoH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "api.themoviedb.org" {
			t.Errorf("name %q", r.URL.Query().Get("name"))
		}
		w.Write([]byte(`{"Status":0,"Answer":[{"type":5,"data":"x.cloudfront.net."},{"type":1,"data":"127.0.0.1"},{"type":1,"data":"13.224.1.2"}]}`))
	}))
	defer srv.Close()
	ips, err := queryDoH(context.Background(), srv.Client(), srv.URL, "api.themoviedb.org")
	if err != nil || len(ips) != 1 || ips[0] != "13.224.1.2" {
		t.Fatalf("%v %v", ips, err)
	}
	if !tmdbHost("api.themoviedb.org") || !tmdbHost("image.tmdb.org") || tmdbHost("localhost") || tmdbHost("eviltmdb.org") {
		t.Fatal("tmdbHost")
	}
}
