// Package library scans folders, watches them for changes (FR-1), recognises
// titles (FR-2) and enriches them with TMDB metadata (FR-3).
package library

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/parse"
	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/tmdb"
)

type Status struct {
	Scanning bool   `json:"scanning"`
	Library  string `json:"library"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Added    int    `json:"added"`
	Movies   int    `json:"movies"`
	Shows    int    `json:"shows"`
	Matching int    `json:"matching"` // titles waiting for TMDB
	Error    string `json:"error,omitempty"`
}

type Scanner struct {
	Store *store.Store
	Probe *probe.Prober
	TMDB  *tmdb.Client
	Bus   *events.Bus

	mu       sync.Mutex
	status   Status
	scanReq  chan int64 // library id, 0 = all
	metaKick chan struct{}
	watcher  *fsnotify.Watcher
}

func New(s *store.Store, p *probe.Prober, t *tmdb.Client, bus *events.Bus) *Scanner {
	return &Scanner{Store: s, Probe: p, TMDB: t, Bus: bus, scanReq: make(chan int64, 16), metaKick: make(chan struct{}, 1)}
}

func (sc *Scanner) Status() Status {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	st := sc.status
	st.Movies, st.Shows = sc.Store.Counts()
	return st
}

func (sc *Scanner) publish() { sc.Bus.Publish("scan", sc.Status()) }

func (sc *Scanner) setStatus(f func(*Status)) {
	sc.mu.Lock()
	f(&sc.status)
	sc.mu.Unlock()
	sc.publish()
}

// Run starts the scan loop, the folder watcher and the metadata worker.
func (sc *Scanner) Run(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err == nil {
		sc.watcher = w
		go sc.watchLoop(ctx)
	} else {
		log.Printf("watcher: %v", err)
	}
	go sc.metaLoop(ctx)
	sc.RequestScan(0)
	// Periodic full rescan catches changes the watcher missed (network drives).
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-sc.scanReq:
			sc.scan(ctx, id)
		case <-ticker.C:
			sc.scan(ctx, 0)
		}
	}
}

func (sc *Scanner) RequestScan(libraryID int64) {
	select {
	case sc.scanReq <- libraryID:
	default:
	}
}

func (sc *Scanner) KickMetadata() {
	select {
	case sc.metaKick <- struct{}{}:
	default:
	}
}

func (sc *Scanner) scan(ctx context.Context, only int64) {
	libs, err := sc.Store.Libraries()
	if err != nil {
		return
	}
	for _, lib := range libs {
		if only != 0 && lib.ID != only {
			continue
		}
		sc.scanLibrary(ctx, lib)
	}
	sc.setStatus(func(s *Status) { s.Scanning = false })
	sc.KickMetadata()
}

func (sc *Scanner) scanLibrary(ctx context.Context, lib store.Library) {
	known, err := sc.Store.AllFilePaths(lib.ID)
	if err != nil {
		return
	}
	var files []string
	walkErr := filepath.WalkDir(lib.Path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable folder: skip, keep scanning
		}
		if d.IsDir() {
			name := d.Name()
			if p != lib.Path && (strings.HasPrefix(name, ".") || strings.EqualFold(name, "$RECYCLE.BIN") || strings.EqualFold(name, "System Volume Information") || name == ".incomplete") {
				return filepath.SkipDir
			}
			if sc.watcher != nil {
				_ = sc.watcher.Add(p)
			}
			return nil
		}
		if parse.IsVideo(p) && !parse.IsSample(p) {
			files = append(files, p)
		}
		return nil
	})
	sc.setStatus(func(s *Status) {
		*s = Status{Scanning: true, Library: lib.Path, Total: len(files)}
		if walkErr != nil {
			s.Error = walkErr.Error()
		}
	})
	if _, err := os.Stat(lib.Path); err != nil {
		// Drive not connected: keep the library as is rather than wiping it.
		sc.setStatus(func(s *Status) { s.Error = "Папка недоступна: " + lib.Path })
		return
	}

	seen := map[string]bool{}
	last := time.Now()
	for i, p := range files {
		if ctx.Err() != nil {
			return
		}
		seen[p] = true
		added, err := sc.indexFile(ctx, lib, p, "folder")
		if err != nil {
			log.Printf("scan %s: %v", p, err)
		}
		if added || time.Since(last) > 500*time.Millisecond || i == len(files)-1 {
			last = time.Now()
			sc.setStatus(func(s *Status) {
				s.Done = i + 1
				if added {
					s.Added++
				}
			})
		}
	}
	for p, id := range known {
		if !seen[p] {
			_ = sc.Store.DeleteFile(id)
		}
	}
}

// indexFile adds a new file or refreshes a changed one. Returns true if added.
func (sc *Scanner) indexFile(ctx context.Context, lib store.Library, path, source string) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	existing, err := sc.Store.FileByPath(path)
	if err != nil {
		return false, err
	}
	if existing != nil {
		if existing.Probed && existing.Size == st.Size() && existing.Mtime == st.ModTime().Unix() {
			return false, nil
		}
		f, _ := sc.Probe.Probe(ctx, path)
		f.ID = existing.ID
		return false, sc.Store.UpdateFileProbe(f)
	}
	if isGrowing(path, st) {
		return false, nil // still being copied; the watcher will see it again
	}
	f, perr := sc.Probe.Probe(ctx, path)
	f.Probed = perr == nil
	r := parse.Parse(path)
	if lib.Kind == "movies" {
		r.Kind = "movie"
		if r.Season > 0 { // keep the full name if the "episode" guess was wrong
			r = parse.Result{Kind: "movie", Title: r.Title, Year: r.Year}
		}
	}
	n := store.NewItem{LibraryID: lib.ID, Kind: r.Kind, Title: r.Title, Year: r.Year, Season: r.Season, Episode: r.Episode, Source: source, File: f}
	if r.Kind == "episode" {
		showID, err := sc.Store.UpsertShow(r.Title, r.Year)
		if err != nil {
			return false, err
		}
		n.ShowID = showID
	}
	if n.Title == "" {
		n.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	_, err = sc.Store.InsertItem(n)
	return err == nil, err
}

// isGrowing treats files modified in the last few seconds as still copying.
func isGrowing(path string, st os.FileInfo) bool {
	if time.Since(st.ModTime()) > 5*time.Second {
		return false
	}
	time.Sleep(2 * time.Second)
	st2, err := os.Stat(path)
	return err != nil || st2.Size() != st.Size()
}

func (sc *Scanner) watchLoop(ctx context.Context) {
	pending := map[int64]bool{}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			sc.watcher.Close()
			return
		case ev, ok := <-sc.watcher.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Chmod) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = sc.watcher.Add(ev.Name)
				}
			}
			if id := sc.libraryFor(ev.Name); id != 0 {
				pending[id] = true
				timer.Reset(5 * time.Second) // debounce bursts of copy events
			}
		case <-timer.C:
			for id := range pending {
				sc.RequestScan(id)
			}
			pending = map[int64]bool{}
		case err := <-sc.watcher.Errors:
			log.Printf("watcher: %v", err)
		}
	}
}

func (sc *Scanner) libraryFor(path string) int64 {
	libs, _ := sc.Store.Libraries()
	for _, l := range libs {
		if rel, err := filepath.Rel(l.Path, path); err == nil && !strings.HasPrefix(rel, "..") {
			return l.ID
		}
	}
	return 0
}

// ---------- metadata ----------

func (sc *Scanner) metaLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		sc.matchPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-sc.metaKick:
		case <-ticker.C:
		}
	}
}

func (sc *Scanner) matchPending(ctx context.Context) {
	for ctx.Err() == nil {
		batch, err := sc.Store.PendingMetadata(20)
		if err != nil || len(batch) == 0 {
			sc.setStatus(func(s *Status) { s.Matching = 0 })
			return
		}
		sc.setStatus(func(s *Status) { s.Matching = len(batch) })
		for _, p := range batch {
			err := sc.matchOne(ctx, p)
			if errors.Is(err, tmdb.ErrNoKey) {
				return // nothing to do until the user sets a key
			}
			if err != nil {
				log.Printf("tmdb %s %q: %v", p.Kind, p.Title, err)
				if isNetErr(err) {
					return // offline: keep items pending, retry later
				}
			}
		}
	}
}

func isNetErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "dial") || strings.Contains(s, "timeout") || strings.Contains(s, "no such host") || strings.Contains(s, "неверный ключ") || strings.Contains(s, "лимит") || strings.Contains(s, "нет связи")
}

func (sc *Scanner) matchOne(ctx context.Context, p store.Pending) error {
	switch p.Kind {
	case "movie":
		res, err := sc.TMDB.Search(ctx, "movie", p.Title, p.Year)
		if err != nil {
			return err
		}
		best := tmdb.BestMatch(res, p.Title, p.Year)
		if best == nil {
			return sc.Store.SetItemMatched(p.ID, 2)
		}
		m, err := sc.TMDB.Details(ctx, "movie", best.ID)
		if err != nil {
			return err
		}
		return sc.Store.ApplyItemMeta(p.ID, m)
	case "show":
		res, err := sc.TMDB.Search(ctx, "tv", p.Title, p.Year)
		if err != nil {
			return err
		}
		best := tmdb.BestMatch(res, p.Title, p.Year)
		if best == nil {
			return sc.Store.SetShowMatched(p.ID, 2)
		}
		m, err := sc.TMDB.Details(ctx, "tv", best.ID)
		if err != nil {
			return err
		}
		return sc.Store.ApplyShowMeta(p.ID, m)
	case "episode":
		m, err := sc.TMDB.Episode(ctx, p.ShowTmdb, p.Season, p.Episode)
		if err != nil {
			if isNetErr(err) {
				return err
			}
			return sc.Store.SetItemMatched(p.ID, 2)
		}
		m.Title = "" // keep the show title on the episode row
		return sc.Store.ApplyItemMeta(p.ID, m)
	}
	return nil
}

// Rematch applies a manually chosen TMDB id (FR-4).
func (sc *Scanner) Rematch(ctx context.Context, kind string, id, tmdbID int64) error {
	if kind == "show" {
		m, err := sc.TMDB.Details(ctx, "tv", tmdbID)
		if err != nil {
			return err
		}
		m.Matched = 3
		if err := sc.Store.ApplyShowMeta(id, m); err != nil {
			return err
		}
		_, err = sc.Store.Exec(`UPDATE media_items SET matched=0 WHERE show_id=?`, id)
		sc.KickMetadata()
		return err
	}
	m, err := sc.TMDB.Details(ctx, "movie", tmdbID)
	if err != nil {
		return err
	}
	m.Matched = 3
	return sc.Store.ApplyItemMeta(id, m)
}

// ImportFile indexes a single file right away (used after a torrent finishes).
func (sc *Scanner) ImportFile(ctx context.Context, path, root, kind string) error {
	id := sc.libraryFor(path)
	if id == 0 && root != "" {
		lib, err := sc.Store.AddLibrary(root, kind)
		if err != nil {
			return err
		}
		id = lib.ID
		if sc.watcher != nil {
			_ = sc.watcher.Add(root)
		}
	}
	if id == 0 {
		return errors.New("файл вне папок библиотеки")
	}
	libs, _ := sc.Store.Libraries()
	for _, l := range libs {
		if l.ID == id {
			_, err := sc.indexFile(ctx, l, path, "torrent")
			sc.KickMetadata()
			sc.publish()
			return err
		}
	}
	return nil
}
