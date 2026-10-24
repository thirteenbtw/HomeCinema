package torrent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

func TestTrackerList(t *testing.T) {
	st := Settings{Trackers: "udp://a.example:1337/announce\r\nhttp://b.example/ann?magnet, junk\n\nmagnet:?x"}
	got := st.trackerList()
	if strings.Join(got, "|") != "udp://a.example:1337/announce|http://b.example/ann?magnet" {
		t.Fatalf("got %q", got)
	}
	if len(DefaultSettings("").trackerList()) != len(DefaultTrackers) {
		t.Fatal("default trackers do not parse")
	}
}

// A magnet added to the engine bound to an interface gets its metadata from
// a peer, reports it is waiting until then, and downloads the file, with and
// without a speed limit.
func TestMagnetMetadataFromPeer(t *testing.T) {
	for _, kbs := range []int{0, 2000} {
		t.Run(fmt.Sprintf("limit=%d", kbs), func(t *testing.T) { testMagnet(t, kbs) })
	}
}

func testMagnet(t *testing.T, downKBs int) {
	dir := t.TempDir()
	// Seeder with a small file.
	seedDir := filepath.Join(dir, "seed")
	_ = os.MkdirAll(seedDir, 0o755)
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i * 7)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "movie.mkv"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 256 << 10}
	if err := info.BuildFromFilePath(filepath.Join(seedDir, "movie.mkv")); err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{}
	mi.InfoBytes, _ = bencode.Marshal(info)
	scfg := torrent.NewDefaultClientConfig()
	scfg.DataDir = seedDir
	scfg.Seed = true
	scfg.NoDHT = true
	scfg.ListenPort = 0
	scfg.DisableIPv6 = true
	scfg.SetListenAddr("127.0.0.1:0")
	seeder, err := torrent.NewClient(scfg)
	if err != nil {
		t.Fatal(err)
	}
	defer seeder.Close()
	st, _ := seeder.AddTorrent(&mi)
	st.VerifyData()

	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ls := DefaultSettings(dir)
	ls.DownloadDir = filepath.Join(dir, "dl")
	ls.Port = 0
	ls.DHT, ls.UPnP = false, false
	ls.Trackers = ""
	ls.DownKBs = downKBs
	ls.BindIface = loopbackIface(t)
	b, _ := json.Marshal(ls)
	_ = db.SetSetting("torrent", string(b))
	e := New(db, events.New(), dir, dir)
	if err := e.Restart(); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	hash, err := e.Add(mi.Magnet(nil, &info).String(), nil, "download", true)
	if err != nil {
		t.Fatal(err)
	}
	if ov := e.List(); len(ov.Items) != 1 || ov.Items[0].Status != "meta" || ov.DHT != -1 {
		t.Fatalf("before peers: %+v", ov)
	}
	e.mu.Lock()
	e.items[hash].t.AddClientPeer(seeder)
	e.mu.Unlock()
	if err := e.WaitInfo(hash, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Get(hash); got.Name != "movie.mkv" || got.FileCount != 1 {
		t.Fatalf("after metadata: %+v", got)
	}
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		e.step()
		if got, _ := e.Get(hash); got.Progress >= 1 {
			break
		}
		if time.Now().After(deadline) {
			got, _ := e.Get(hash)
			t.Fatalf("download did not finish: %+v", got)
		}
	}
}

func loopbackIface(t *testing.T) string {
	if _, err := ifaceIPv4("lo"); err == nil {
		return "lo"
	}
	t.Skip("no loopback interface named lo")
	return ""
}
