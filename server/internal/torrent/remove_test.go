package torrent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/library"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// Deleting a finished torrent with its data removes the download, the library
// copy (a hard link of the same file) with its subtitles and the emptied
// folders, and leaves an unrelated film of the same name alone.
func TestRemoveDeletesLibraryCopy(t *testing.T) {
	dir := t.TempDir()
	dl := filepath.Join(dir, "dl")
	movies, shows := filepath.Join(dir, "Movies"), filepath.Join(dir, "Shows")
	const name = "Some.Film.2021.1080p.mkv"
	src := filepath.Join(dl, name)
	_ = os.MkdirAll(dl, 0o755)
	if err := os.WriteFile(src, bytes.Repeat([]byte("hc"), 300<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := library.Target(name, movies, shows)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := os.Link(src, dst); err != nil {
		t.Skip("no hard links here")
	}
	sub := dst[:len(dst)-len(".mkv")] + ".rus.srt"
	_ = os.WriteFile(sub, []byte("1"), 0o644)
	other := library.Target("Other.Film.2020.mkv", movies, shows)
	_ = os.MkdirAll(filepath.Dir(other), 0o755)
	_ = os.WriteFile(other, []byte("x"), 0o644)

	info := metainfo.Info{PieceLength: 64 << 10}
	if err := info.BuildFromFilePath(src); err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{}
	mi.InfoBytes, _ = bencode.Marshal(info)
	var tf bytes.Buffer
	if err := mi.Write(&tf); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ls := DefaultSettings(dir)
	ls.DownloadDir, ls.MoviesDir, ls.ShowsDir = dl, movies, shows
	ls.Port = 0
	ls.DHT, ls.UPnP, ls.AutoImport = false, false, false
	ls.Trackers = ""
	b, _ := json.Marshal(ls)
	_ = db.SetSetting("torrent", string(b))
	e := New(db, events.New(), dir, dir)
	if err := e.Restart(); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	hash, err := e.Add("", tf.Bytes(), "download", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.WaitInfo(hash, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(hash, true, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{src, dst, sub, filepath.Dir(dst)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("unrelated film deleted: %v", err)
	}
}
