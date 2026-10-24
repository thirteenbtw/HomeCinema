package api

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/torrent"
)

func (s *Server) listTorrents(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.Torrents.List())
}

// addTorrent accepts JSON {magnet, mode, keep} or a multipart .torrent upload (FR-T1).
func (s *Server) addTorrent(w http.ResponseWriter, r *http.Request) error {
	if !s.Torrents.Settings().AcceptedRisk {
		return httpError{http.StatusPreconditionRequired, "подтвердите предупреждение об ответственности за контент"}
	}
	var source, mode, link string
	var file []byte
	keep := true
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			return badRequest("%v", err)
		}
		mode = r.FormValue("mode")
		keep = r.FormValue("keep") != "0"
		source = r.FormValue("magnet")
		if fh, _, err := r.FormFile("file"); err == nil {
			file, _ = io.ReadAll(io.LimitReader(fh, 16<<20))
			fh.Close()
		}
	} else {
		var b struct {
			Magnet string `json:"magnet"`
			Link   string `json:"link"` // .torrent URL from a search result
			Mode   string `json:"mode"`
			Keep   *bool  `json:"keep"`
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		source, mode, link = b.Magnet, b.Mode, b.Link
		if b.Keep != nil {
			keep = *b.Keep
		}
	}
	if link != "" {
		if strings.HasPrefix(link, "magnet:") {
			source = link
		} else {
			// The .torrent from the indexer skips the metadata wait; the
			// magnet sent along (if any) is the fallback.
			data, magnet, err := torrent.FetchTorrent(r.Context(), link)
			switch {
			case err == nil && magnet != "":
				source, file = magnet, nil
			case err == nil:
				file = data
			case !strings.HasPrefix(source, "magnet:"):
				return badRequest("не удалось скачать .torrent: %v", err)
			}
		}
	}
	if source == "" && len(file) == 0 {
		return badRequest("нужна magnet-ссылка или .torrent-файл")
	}
	hash, err := s.Torrents.Add(source, file, mode, keep)
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]string{"hash": hash})
}

func (s *Server) getTorrent(w http.ResponseWriter, r *http.Request) error {
	info, err := s.Torrents.Get(r.PathValue("hash"))
	if err != nil {
		return notFound(err.Error())
	}
	return writeJSON(w, info)
}

func (s *Server) patchTorrent(w http.ResponseWriter, r *http.Request) error {
	hash := r.PathValue("hash")
	var b struct {
		Paused   *bool  `json:"paused"`
		Priority *int   `json:"priority"`
		Mode     string `json:"mode"`
		Keep     *bool  `json:"keep"`
		FilesOff *[]int `json:"filesOff"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	var err error
	if b.Paused != nil {
		err = s.Torrents.SetPaused(hash, *b.Paused)
	}
	if err == nil && b.Priority != nil {
		err = s.Torrents.SetPriority(hash, *b.Priority)
	}
	if err == nil && (b.Mode != "" || b.Keep != nil) {
		err = s.Torrents.SetMode(hash, b.Mode, b.Keep)
	}
	if err == nil && b.FilesOff != nil {
		err = s.Torrents.SetFiles(hash, *b.FilesOff)
	}
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) deleteTorrent(w http.ResponseWriter, r *http.Request) error {
	if err := s.Torrents.Remove(r.PathValue("hash"), r.URL.Query().Get("data") == "1"); err != nil {
		return notFound(err.Error())
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) torrentStream(w http.ResponseWriter, r *http.Request) error {
	st, err := s.Torrents.Stream(r.PathValue("hash"), queryInt(r, "file", -1), queryInt(r, "cells", 200))
	if err != nil {
		return notFound(err.Error())
	}
	return writeJSON(w, st)
}

func (s *Server) torrentFile(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		idx = -1
	}
	s.Torrents.ServeFile(w, r, r.PathValue("hash"), idx)
}

// torrentFileID gives a torrent file a stable pseudo file id (negative, so it
// never collides with library files) for transcode session bookkeeping.
func torrentFileID(hash string, idx int) int64 {
	h := sha1.Sum([]byte(fmt.Sprintf("%s/%d", strings.ToLower(hash), idx)))
	return -int64(binary.BigEndian.Uint64(h[:8]) >> 2)
}

func (s *Server) loopbackBase() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port)
}

// torrentPlayback prepares a torrent file for playback while it downloads
// (FR-T4): direct when the client can decode it, otherwise HLS over the
// partially downloaded file (FR-T5).
func (s *Server) torrentPlayback(ctx context.Context, hash string, idx int, caps rendererCaps, q map[string][]string) (*playback, error) {
	if err := s.Torrents.WaitInfo(hash, 60*time.Second); err != nil {
		return nil, badRequest("%v", err)
	}
	idx, name, size, err := s.Torrents.FileIndex(hash, idx)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	if torrent.Blocked(name) {
		return nil, badRequest("исполняемые файлы не открываются")
	}
	input := s.loopbackBase() + "/api/t/" + s.streamToken("torrent:"+strings.ToLower(hash)) + fmt.Sprintf("/torrents/%s/files/%d", strings.ToLower(hash), idx)
	if p, ok := s.Torrents.LocalPath(hash, idx); ok {
		input = p
	}
	f, err := s.probeURL(ctx, fmt.Sprintf("%s/%d", hash, idx), input)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	f.ID = torrentFileID(hash, idx)
	f.Size = size
	f.Container = containerFromName(name, f.Container)
	direct := fmt.Sprintf("/api/torrents/%s/files/%d", strings.ToLower(hash), idx)
	pb, err := s.preparePlayback(ctx, f, input, caps, q, "", direct)
	if err != nil {
		return nil, err
	}
	pb.Subs = []trackOut{} // subtitles of streams in progress are not extracted
	pb.Title = name
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		pb.Title = name[i+1:]
	}
	pb.Info = "Торрент · " + pb.Summary
	return pb, nil
}

func containerFromName(name, probed string) string {
	switch strings.ToLower(name[strings.LastIndex(name, ".")+1:]) {
	case "mp4", "m4v":
		return "mp4"
	case "webm":
		return "webm"
	case "mkv":
		return "mkv"
	}
	return probed
}

func (s *Server) torrentPlay(w http.ResponseWriter, r *http.Request) error {
	pb, err := s.torrentPlayback(r.Context(), r.PathValue("hash"), queryInt(r, "file", -1), browserCaps(r), r.URL.Query())
	if err != nil {
		return err
	}
	return writeJSON(w, pb)
}

func (s *Server) searchTorrents(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return badRequest("пустой запрос")
	}
	if len(s.Torrents.Indexers()) == 0 {
		return badRequest("включите источник поиска (JacRed или свой Torznab) в настройках")
	}
	res, errs := s.Torrents.Search(r.Context(), q)
	return writeJSON(w, map[string]any{"results": res, "errors": errs})
}

func (s *Server) getIndexers(w http.ResponseWriter, r *http.Request) error {
	list := s.Torrents.Indexers()
	for i := range list {
		if list[i].APIKey != "" {
			list[i].APIKey = "••••" + tail(list[i].APIKey, 4)
		}
	}
	return writeJSON(w, map[string]any{"indexers": list, "prefs": s.Torrents.Prefs()})
}

func (s *Server) putIndexers(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		Indexers []torrent.Indexer    `json:"indexers"`
		Prefs    *torrent.SearchPrefs `json:"prefs"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	old := map[string]string{}
	for _, in := range s.Torrents.Indexers() {
		old[in.URL] = in.APIKey
	}
	for i := range b.Indexers {
		if strings.HasPrefix(b.Indexers[i].APIKey, "••••") { // unchanged masked key
			b.Indexers[i].APIKey = old[b.Indexers[i].URL]
		}
	}
	if err := s.Torrents.SetIndexers(b.Indexers); err != nil {
		return err
	}
	if b.Prefs != nil {
		_ = s.Store.SetSetting("searchPrefs", toJSONString(b.Prefs))
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) getTorrentSettings(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.Torrents.Settings())
}

func (s *Server) putTorrentSettings(w http.ResponseWriter, r *http.Request) error {
	st := s.Torrents.Settings()
	if err := readJSON(r, &st); err != nil {
		return err
	}
	if st.Port < 1024 || st.Port > 65535 {
		return badRequest("порт должен быть в диапазоне 1024–65535")
	}
	if err := s.Torrents.UpdateSettings(st); err != nil {
		return badRequest("%v", err)
	}
	// Make sure finished downloads land in a folder the library watches.
	for _, dir := range []struct{ path, kind string }{{st.MoviesDir, "movies"}, {st.ShowsDir, "shows"}} {
		if dir.path != "" {
			if lib, err := s.Store.AddLibrary(dir.path, dir.kind); err == nil {
				s.Scanner.RequestScan(lib.ID)
			}
		}
	}
	return writeJSON(w, s.Torrents.Settings())
}

func (s *Server) interfaces(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, torrent.Interfaces())
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

var _ = store.File{}
