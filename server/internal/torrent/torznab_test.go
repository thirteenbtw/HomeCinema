package torrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A result keeps the .torrent link (no metadata wait) and the magnet as a
// fallback; a magnet-only result uses the magnet as its link.
func TestTorznabKeepsTorrentAndMagnet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>Film 2160p</title><link>http://jackett/dl/1</link>
<torznab:attr name="magneturl" value="magnet:?xt=urn:btih:aaaa&amp;tr=http://t/ann"/><torznab:attr name="seeders" value="9"/></item>
<item><title>Film 1080p</title><torznab:attr name="infohash" value="BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"/></item>
</channel></rss>`))
	}))
	defer srv.Close()
	res, err := queryTorznab(context.Background(), &http.Client{Timeout: 5 * time.Second}, Indexer{Name: "x", URL: srv.URL}, "film")
	if err != nil || len(res) != 2 {
		t.Fatalf("%v %+v", err, res)
	}
	if res[0].Link != "http://jackett/dl/1" || res[0].Magnet != "magnet:?xt=urn:btih:aaaa&tr=http://t/ann" || res[0].Seeds != 9 {
		t.Errorf("first: %+v", res[0])
	}
	if res[1].Link != "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Film+1080p" || res[1].Magnet != "" {
		t.Errorf("second: %+v", res[1])
	}
}
