package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/letterboxd"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// Likes, "watch later" and "watched" for catalog titles, plus Letterboxd:
// import of its data export, a sync from the member's RSS feed, export in
// its import format, and IMDb / Letterboxd scores on title pages.

func (s *Server) listMarks(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	l := r.URL.Query().Get("list")
	if l != "" && !store.ValidMarkList(l) {
		return badRequest("неизвестный список")
	}
	m, err := s.Store.Marks(pid, l)
	if err != nil {
		return err
	}
	return writeJSON(w, m)
}

// markKeys lists which titles are in which list ("movie:123"), so cards
// across the app can show a heart without loading whole lists.
func (s *Server) markKeys(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	all, err := s.Store.Marks(pid, "")
	if err != nil {
		return err
	}
	out := map[string]map[string]int{"liked": {}, "later": {}, "watched": {}}
	for _, m := range all {
		out[m.List][m.Kind+":"+strconv.FormatInt(m.ID, 10)] = m.Rating
	}
	return writeJSON(w, out)
}

func (s *Server) setMark(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	var b struct {
		store.Mark
		On bool `json:"on"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if b.ID <= 0 {
		return badRequest("нужен id TMDB")
	}
	b.Source, b.AddedAt = "", 0
	if b.Rating < 0 || b.Rating > 10 {
		return badRequest("оценка от 1 до 10")
	}
	if err := s.Store.SetMark(pid, b.Mark, b.On); err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) catalogRatings(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	kind := catalogKind(r, "kind")
	t, err := s.TMDB.Title(r.Context(), kind, id)
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, s.ratings.Get(r.Context(), kind, id, t.ImdbID))
}

// ---------- Letterboxd ----------

type lbJob struct {
	Running   bool     `json:"running"`
	ProfileID int64    `json:"profileId"`
	Kind      string   `json:"kind"` // import | sync
	Done      int      `json:"done"`
	Total     int      `json:"total"`
	Found     int      `json:"found"`
	Missed    []string `json:"missed"` // films TMDB did not find (first 100)
	Error     string   `json:"error,omitempty"`
	At        int64    `json:"at"`
}

type lbState struct {
	mu  sync.Mutex
	job lbJob
}

func lbUserKey(pid int64) string { return fmt.Sprintf("letterboxd.user.%d", pid) }

func (s *Server) letterboxdState(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	s.lb.mu.Lock()
	job := s.lb.job
	s.lb.mu.Unlock()
	var jp *lbJob
	if job.ProfileID == pid {
		jp = &job
	}
	synced, _ := strconv.ParseInt(s.Store.Setting(fmt.Sprintf("letterboxd.synced.%d", pid), "0"), 10, 64)
	return writeJSON(w, map[string]any{"user": s.Store.Setting(lbUserKey(pid), ""), "syncedAt": synced, "job": jp})
}

func (s *Server) putLetterboxd(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	var b struct {
		User string `json:"user"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	u := ""
	if strings.TrimSpace(b.User) != "" {
		if u = letterboxd.User(b.User); u == "" {
			return badRequest("это не имя пользователя Letterboxd")
		}
	}
	if err := s.Store.SetSetting(lbUserKey(pid), u); err != nil {
		return err
	}
	if u != "" {
		s.startLB(pid, "sync", func(ctx context.Context) ([]letterboxd.Entry, error) {
			return letterboxd.FetchRSS(ctx, s.lbHTTP, u)
		})
	}
	return writeJSON(w, map[string]any{"ok": true, "user": u})
}

func (s *Server) syncLetterboxd(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	u := s.Store.Setting(lbUserKey(pid), "")
	if u == "" {
		return badRequest("сначала укажите имя пользователя Letterboxd")
	}
	if !s.startLB(pid, "sync", func(ctx context.Context) ([]letterboxd.Entry, error) { return letterboxd.FetchRSS(ctx, s.lbHTTP, u) }) {
		return httpError{http.StatusConflict, "импорт уже идёт"}
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) importLetterboxd(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return badRequest("%v", err)
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest("нужен файл экспорта Letterboxd (ZIP или CSV)")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return err
	}
	fallback := r.FormValue("list")
	if fallback != "" && !store.ValidMarkList(fallback) {
		return badRequest("неизвестный список")
	}
	entries, err := letterboxd.ParseFile(hdr.Filename, data, fallback)
	if err == letterboxd.ErrUnknownFile {
		return httpError{http.StatusUnprocessableEntity, "Не похоже на экспорт Letterboxd. Загрузите ZIP из Settings → Data → Export your data, или выберите, в какой список положить этот CSV."}
	}
	if err != nil {
		return badRequest("%v", err)
	}
	if len(entries) == 0 {
		return badRequest("в файле нет фильмов")
	}
	if !s.startLB(pid, "import", func(context.Context) ([]letterboxd.Entry, error) { return entries, nil }) {
		return httpError{http.StatusConflict, "импорт уже идёт"}
	}
	return writeJSON(w, map[string]any{"ok": true, "total": len(entries)})
}

func (s *Server) clearLetterboxd(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	n, err := s.Store.ClearMarks(pid, "letterboxd")
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"ok": true, "removed": n})
}

// exportLetterboxd writes a CSV for letterboxd.com/import: "films" holds
// what the profile watched or liked (with ratings), "watchlist" the
// watch-later list. Letterboxd only knows films, so shows are left out.
func (s *Server) exportLetterboxd(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(r.PathValue("name"), ".csv")
	if name != "films" && name != "watchlist" {
		return notFound("нет такого файла")
	}
	all, err := s.Store.Marks(pid, "")
	if err != nil {
		return err
	}
	var films []letterboxd.Film
	idx := map[int64]int{}
	for i := len(all) - 1; i >= 0; i-- { // oldest first, as a diary reads
		m := all[i]
		if m.Kind != "movie" || (name == "watchlist") != (m.List == "later") {
			continue
		}
		j, ok := idx[m.ID]
		if !ok {
			j = len(films)
			idx[m.ID] = j
			films = append(films, letterboxd.Film{TMDB: m.ID, Title: firstNonEmpty(m.Orig, m.Title), Year: m.Year})
		}
		f := &films[j]
		if m.Rating > 0 {
			f.Rating = m.Rating
		}
		switch m.List {
		case "liked":
			f.Liked = true
		case "watched":
			f.Watched = m.AddedAt
		}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="homecinema-letterboxd-%s.csv"`, name))
	return letterboxd.WriteCSV(w, films)
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// startLB runs one import or sync at a time in the background, publishing
// progress as "letterboxd" events. It reports false when one is running.
func (s *Server) startLB(pid int64, kind string, load func(context.Context) ([]letterboxd.Entry, error)) bool {
	s.lb.mu.Lock()
	if s.lb.job.Running {
		s.lb.mu.Unlock()
		return false
	}
	s.lb.job = lbJob{Running: true, ProfileID: pid, Kind: kind, Missed: []string{}, At: time.Now().Unix()}
	s.lb.mu.Unlock()
	s.publishLB()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		err := s.runLB(ctx, pid, kind, load)
		s.lb.mu.Lock()
		s.lb.job.Running = false
		if err != nil {
			s.lb.job.Error = err.Error()
			log.Printf("letterboxd %s: %v", kind, err)
		}
		s.lb.mu.Unlock()
		if err == nil && kind == "sync" {
			_ = s.Store.SetSetting(fmt.Sprintf("letterboxd.synced.%d", pid), strconv.FormatInt(time.Now().Unix(), 10))
		}
		s.publishLB()
	}()
	return true
}

func (s *Server) publishLB() {
	s.lb.mu.Lock()
	job := s.lb.job
	job.Missed = append([]string{}, job.Missed...)
	s.lb.mu.Unlock()
	s.Bus.Publish("letterboxd", job)
}

func (s *Server) runLB(ctx context.Context, pid int64, kind string, load func(context.Context) ([]letterboxd.Entry, error)) error {
	entries, err := load(ctx)
	if err != nil {
		return err
	}
	s.lb.mu.Lock()
	s.lb.job.Total = len(entries)
	s.lb.mu.Unlock()
	s.publishLB()

	type res struct {
		e     letterboxd.Entry
		m     store.Mark
		found bool
		err   error
	}
	in := make(chan letterboxd.Entry)
	out := make(chan res)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range in {
				m, found, err := s.matchLB(ctx, e)
				out <- res{e, m, found, err}
			}
		}()
	}
	go func() {
		defer close(in)
		for _, e := range entries {
			select {
			case in <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	var firstErr error
	last := time.Now()
	for x := range out {
		if x.err != nil && firstErr == nil {
			firstErr = x.err
		}
		if x.found {
			m := x.m
			m.Rating, m.Source, m.AddedAt = x.e.Rating, "letterboxd", x.e.Date
			for list, on := range map[string]bool{"watched": x.e.Watched, "liked": x.e.Liked, "later": x.e.Later} {
				if on {
					m.List = list
					if err := s.Store.SetMark(pid, m, true); err != nil && firstErr == nil {
						firstErr = err
					}
				}
			}
			// a film logged as watched leaves the watchlist, as on Letterboxd
			if x.e.Watched && !x.e.Later && kind == "sync" {
				m.List = "later"
				_ = s.Store.SetMark(pid, m, false)
			}
		}
		s.lb.mu.Lock()
		s.lb.job.Done++
		if x.found {
			s.lb.job.Found++
		} else if len(s.lb.job.Missed) < 100 {
			name := x.e.Name
			if x.e.Year > 0 {
				name += fmt.Sprintf(" (%d)", x.e.Year)
			}
			s.lb.job.Missed = append(s.lb.job.Missed, name)
		}
		s.lb.mu.Unlock()
		if time.Since(last) > 400*time.Millisecond {
			s.publishLB()
			last = time.Now()
		}
	}
	s.lb.mu.Lock()
	found := s.lb.job.Found
	s.lb.mu.Unlock()
	if found == 0 && firstErr != nil {
		return fmt.Errorf("TMDB не отвечает: %w", firstErr)
	}
	return nil
}

// matchLB finds the TMDB title of a Letterboxd entry: the RSS feed names it,
// an export only has the name and year, which are searched once and
// remembered.
func (s *Server) matchLB(ctx context.Context, e letterboxd.Entry) (store.Mark, bool, error) {
	if e.TMDB > 0 {
		m := store.Mark{Kind: e.Kind, ID: e.TMDB, Title: e.Name, Year: e.Year}
		if t, err := s.TMDB.Title(ctx, e.Kind, e.TMDB); err == nil {
			m.Title, m.Orig, m.Year, m.Poster = t.Title, t.Orig, t.Year, t.Poster
		}
		return m, true, nil
	}
	if m, ok := s.Store.LBMatch(e.Name, e.Year); ok {
		return m, m.ID > 0, nil
	}
	it, ok, err := s.TMDB.FindMovie(ctx, e.Name, e.Year)
	if err != nil {
		return store.Mark{}, false, err
	}
	m := store.Mark{Kind: "movie"}
	if ok {
		m = store.Mark{Kind: "movie", ID: it.ID, Title: it.Title, Orig: it.Orig, Year: it.Year, Poster: it.Poster}
	}
	_ = s.Store.SetLBMatch(e.Name, e.Year, m)
	return m, ok, nil
}

// letterboxdLoop pulls every profile's Letterboxd feed a few times a day.
func (s *Server) letterboxdLoop() {
	time.Sleep(2 * time.Minute)
	for {
		profiles, _ := s.Store.Profiles()
		for _, p := range profiles {
			u := s.Store.Setting(lbUserKey(p.ID), "")
			if u == "" {
				continue
			}
			for !s.startLB(p.ID, "sync", func(ctx context.Context) ([]letterboxd.Entry, error) { return letterboxd.FetchRSS(ctx, s.lbHTTP, u) }) {
				time.Sleep(time.Minute)
			}
			for s.lbRunning() {
				time.Sleep(5 * time.Second)
			}
		}
		time.Sleep(6 * time.Hour)
	}
}

func (s *Server) lbRunning() bool {
	s.lb.mu.Lock()
	defer s.lb.mu.Unlock()
	return s.lb.job.Running
}
