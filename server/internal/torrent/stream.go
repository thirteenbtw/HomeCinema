package torrent

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"

	"github.com/thirteenbtw/homecinema/server/internal/library"
)

// streamReader wraps a torrent reader so the engine knows where the player is
// reading: that position drives the buffer indicator and the piece map.
type streamReader struct {
	torrent.Reader
	e   *Engine
	m   *managed
	pos int64
}

func (r *streamReader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	r.e.mu.Lock()
	r.pos += int64(n)
	r.m.streamPos = r.pos
	r.e.mu.Unlock()
	return n, err
}

func (r *streamReader) Seek(off int64, whence int) (int64, error) {
	p, err := r.Reader.Seek(off, whence)
	if err == nil {
		r.e.mu.Lock()
		r.pos = p
		r.m.streamPos = p
		r.e.mu.Unlock()
	}
	return p, err
}

func (r *streamReader) Close() error {
	r.e.mu.Lock()
	delete(r.m.streams, r)
	r.e.mu.Unlock()
	return r.Reader.Close()
}

// openStream returns a reader for one file of a torrent, with sequential
// readahead (FR-T4). Seeking moves the high-priority window with it.
func (e *Engine) openStream(hash string, idx int) (*streamReader, *torrent.File, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return nil, nil, err
	}
	if m.t.Info() == nil {
		return nil, nil, errors.New("метаданные раздачи ещё не получены")
	}
	files := m.t.Files()
	if idx < 0 {
		idx = largestVideo(files)
	}
	if idx < 0 || idx >= len(files) {
		return nil, nil, errors.New("файл не найден в раздаче")
	}
	f := files[idx]
	if Blocked(f.Path()) {
		return nil, nil, errors.New("исполняемые файлы из раздач не открываются")
	}
	// The container index (mp4 moov atom, mkv cues) is often at the end.
	pieceLen := m.t.Info().PieceLength
	end := f.EndPieceIndex()
	tailPieces := int(max(4<<20/pieceLen, 1))
	for i := max(end-tailPieces, f.BeginPieceIndex()); i < end; i++ {
		m.t.Piece(i).SetPriority(torrent.PiecePriorityHigh)
	}
	rd := f.NewReader()
	rd.SetResponsive()
	rd.SetReadahead(32 << 20)
	sr := &streamReader{Reader: rd, e: e, m: m}
	m.streams[sr] = struct{}{}
	m.streamIdx = idx
	m.t.AllowDataDownload()
	return sr, f, nil
}

func largestVideo(files []*torrent.File) int {
	best, size := -1, int64(0)
	for i, f := range files {
		if Blocked(f.Path()) {
			continue
		}
		ext := path.Ext(f.Path())
		if isVideoExt(ext) && f.Length() > size {
			best, size = i, f.Length()
		}
	}
	return best
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".webm", ".ts", ".m2ts", ".wmv", ".mpg":
		return true
	}
	return false
}

// ServeFile streams a torrent file over HTTP with Range support, so both the
// PC player and the TV can seek (FR-T4, FR-T5).
func (e *Engine) ServeFile(w http.ResponseWriter, r *http.Request, hash string, idx int) {
	sr, f, err := e.openStream(hash, idx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer sr.Close()
	sr.SetContext(r.Context())
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, path.Base(f.Path()), time.Time{}, sr)
}

// FileIndex resolves -1 to the main video file.
func (e *Engine) FileIndex(hash string, idx int) (int, string, int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return 0, "", 0, err
	}
	if m.t.Info() == nil {
		return 0, "", 0, errors.New("метаданные ещё не получены")
	}
	files := m.t.Files()
	if idx < 0 {
		idx = largestVideo(files)
	}
	if idx < 0 || idx >= len(files) {
		return 0, "", 0, errors.New("в раздаче нет видео")
	}
	return idx, files[idx].DisplayPath(), files[idx].Length(), nil
}

// WaitInfo blocks until the torrent's metadata arrives.
func (e *Engine) WaitInfo(hash string, timeout time.Duration) error {
	e.mu.Lock()
	m, err := e.get(hash)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case <-m.t.GotInfo():
		return nil
	case <-time.After(timeout):
		return errors.New("не удалось получить метаданные раздачи — мало пиров?")
	}
}

type StreamStatus struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	FileIndex int     `json:"fileIndex"`
	FileName  string  `json:"fileName"`
	Size      int64   `json:"size"`
	Buffered  int64   `json:"buffered"` // contiguous bytes ready after the read position
	Target    int64   `json:"target"`
	Ready     bool    `json:"ready"`
	Position  float64 `json:"position"` // 0..1 of the file
	Down      float64 `json:"down"`
	Seeds     int     `json:"seeds"`
	Peers     int     `json:"peers"`
	Pieces    []int   `json:"pieces"` // 0 none, 1 have, 2 priority (player position), 3 partial
	NumPieces int     `json:"numPieces"`
	PieceSize int64   `json:"pieceSize"`
	Progress  float64 `json:"progress"`
}

// Stream reports buffering and a compressed piece map for one file.
func (e *Engine) Stream(hash string, idx int, cells int) (StreamStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return StreamStatus{}, err
	}
	st := StreamStatus{Hash: m.row.InfoHash, Name: m.row.Name, Down: m.down}
	s := m.t.Stats()
	st.Seeds, st.Peers = s.ConnectedSeeders, s.ActivePeers
	if m.t.Info() == nil {
		return st, nil
	}
	files := m.t.Files()
	if idx < 0 {
		idx = m.streamIdx
		if idx < 0 {
			idx = largestVideo(files)
		}
	}
	if idx < 0 || idx >= len(files) {
		return st, errors.New("файл не найден")
	}
	f := files[idx]
	st.FileIndex, st.FileName, st.Size = idx, f.DisplayPath(), f.Length()
	pieceLen := m.t.Info().PieceLength
	st.PieceSize = pieceLen
	begin, end := f.BeginPieceIndex(), f.EndPieceIndex()
	st.NumPieces = end - begin
	if f.Length() > 0 {
		st.Progress = float64(f.BytesCompleted()) / float64(f.Length())
	}
	pos := int64(0)
	if m.streamIdx == idx {
		pos = m.streamPos
	}
	if st.Size > 0 {
		st.Position = float64(pos) / float64(st.Size)
	}
	// Contiguous completed bytes from the read position.
	cur := int((f.Offset() + pos) / pieceLen)
	var buffered int64
	for i := max(cur, begin); i < end; i++ {
		if !m.t.Piece(i).State().Complete {
			break
		}
		buffered += pieceLen
	}
	st.Buffered = min(buffered, st.Size-pos)
	set := e.settings
	st.Target = int64(set.BufferMB) << 20
	if pct := int64(float64(st.Size) * set.BufferPct / 100); pct > 0 && pct < st.Target {
		st.Target = pct
	}
	st.Target = min(st.Target, st.Size-pos)
	st.Ready = st.Buffered >= st.Target || st.Progress >= 1

	if cells <= 0 {
		cells = 200
	}
	n := end - begin
	cells = min(cells, max(n, 1))
	st.Pieces = make([]int, cells)
	for c := 0; c < cells; c++ {
		from := begin + c*n/cells
		to := begin + (c+1)*n/cells
		if to <= from {
			to = from + 1
		}
		have, prio, partial := 0, false, false
		for i := from; i < to && i < end; i++ {
			ps := m.t.Piece(i).State()
			if ps.Complete {
				have++
			} else if ps.Partial {
				partial = true
			}
			if ps.Priority >= torrent.PiecePriorityReadahead {
				prio = true
			}
		}
		switch {
		case have == to-from:
			st.Pieces[c] = 1
		case prio:
			st.Pieces[c] = 2
		case partial || have > 0:
			st.Pieces[c] = 3
		}
	}
	return st, nil
}

// LocalPath returns the on-disk path of a completed torrent file, so finished
// files can be played without going through the torrent reader.
func (e *Engine) LocalPath(hash string, idx int) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil || m.t.Info() == nil {
		return "", false
	}
	files := m.t.Files()
	if idx < 0 || idx >= len(files) {
		return "", false
	}
	f := files[idx]
	if f.BytesCompleted() < f.Length() {
		return "", false
	}
	p := filepath.Join(e.settings.DownloadDir, filepath.FromSlash(f.Path()))
	_, err = os.Stat(p)
	return p, err == nil
}

// placeFile hard-links a finished file into the library layout (falls back to
// a copy across drives), keeping the original for seeding.
func placeFile(src, movies, shows string) (string, error) {
	tmp := src + ".hc-link"
	_ = os.Remove(tmp)
	if err := os.Link(src, tmp); err != nil {
		// Different volume: copy, then organise the copy.
		if err := copyFile(src, tmp); err != nil {
			return "", err
		}
	}
	dst, err := library.OrganizeAs(tmp, src, movies, shows)
	_ = os.Remove(tmp) // gone after a successful move; left over if already imported
	return dst, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
