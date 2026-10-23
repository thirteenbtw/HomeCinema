package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/netutil"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// ---------- session ----------

func (s *Server) state(w http.ResponseWriter, r *http.Request) error {
	profiles, err := s.Store.Profiles()
	if err != nil {
		return err
	}
	lan := netutil.LANAddrs()
	urls := []string{}
	for _, ip := range lan {
		urls = append(urls, "http://"+ip+":"+itoa(s.Port))
	}
	authed := s.authorized(r)
	out := map[string]any{
		"version":      s.Version,
		"setupDone":    s.Store.Setting("setupDone", "") == "1",
		"authRequired": !authed,
		"lanUrls":      urls,
		"uiLang":       s.Store.Setting("uiLang", "ru"),
		"local":        netutil.IsLoopback(r),
	}
	if authed {
		out["profiles"] = profiles
		out["profileId"] = s.profileID(r)
	}
	return writeJSON(w, out)
}

func hashSecret(salt, v string) string {
	h := sha256.Sum256([]byte(salt + "|" + v))
	return hex.EncodeToString(h[:])
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Password string }
	if err := readJSON(r, &body); err != nil {
		return err
	}
	want := s.Store.Setting("password", "")
	if want == "" || hashSecret(hex.EncodeToString(s.secret), body.Password) != want {
		time.Sleep(500 * time.Millisecond)
		return httpError{http.StatusUnauthorized, "неверный пароль"}
	}
	http.SetCookie(w, &http.Cookie{Name: "hc_auth", Value: s.sign("auth", "1", 90*24*time.Hour), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 90 * 24 * 3600})
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{Name: "hc_auth", Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "hc_profile", Value: "", Path: "/", MaxAge: -1})
	return writeJSON(w, map[string]bool{"ok": true})
}

// ---------- profiles (FR-19) ----------

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) error {
	p, err := s.Store.Profiles()
	if err != nil {
		return err
	}
	return writeJSON(w, p)
}

type profileBody struct {
	Name  string          `json:"name"`
	Color string          `json:"color"`
	PIN   *string         `json:"pin"`
	Kid   *bool           `json:"isKid"`
	Prefs json.RawMessage `json:"prefs"`
}

func (s *Server) pinHash(pin string) string {
	if pin == "" {
		return ""
	}
	return hashSecret(hex.EncodeToString(s.secret)+"pin", pin)
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) error {
	var b profileBody
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if strings.TrimSpace(b.Name) == "" {
		return badRequest("укажите имя")
	}
	pin := ""
	if b.PIN != nil {
		pin = s.pinHash(*b.PIN)
	}
	kid := b.Kid != nil && *b.Kid
	if b.Color == "" {
		b.Color = "#2563EB"
	}
	id, err := s.Store.CreateProfile(strings.TrimSpace(b.Name), b.Color, pin, kid)
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]int64{"id": id})
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b profileBody
	if err := readJSON(r, &b); err != nil {
		return err
	}
	// Editing another profile requires being in it (or having no PIN on it).
	if cur := s.profileID(r); cur != id {
		if pin, _ := s.Store.ProfilePIN(id); pin != "" {
			return httpError{http.StatusForbidden, "сначала войдите в этот профиль"}
		}
	}
	var pin *string
	if b.PIN != nil {
		h := s.pinHash(*b.PIN)
		pin = &h
	}
	if err := s.Store.UpdateProfile(id, strings.TrimSpace(b.Name), b.Color, b.Prefs, pin, b.Kid); err != nil {
		return err
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.Store.DeleteProfile(id); err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) selectProfile(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct{ PIN string }
	_ = json.NewDecoder(r.Body).Decode(&b)
	want, err := s.Store.ProfilePIN(id)
	if err != nil {
		return notFound("профиль не найден")
	}
	if want != "" && s.pinHash(b.PIN) != want {
		time.Sleep(400 * time.Millisecond)
		return httpError{http.StatusForbidden, "неверный PIN"}
	}
	http.SetCookie(w, &http.Cookie{Name: "hc_profile", Value: s.sign("profile", itoa(int(id)), 365*24*time.Hour), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 3600})
	return writeJSON(w, map[string]bool{"ok": true})
}

// ---------- browsing ----------

func (s *Server) home(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	featured, err := s.Store.Featured(pid, 5)
	if err != nil {
		return err
	}
	cont, _ := s.Store.Continue(pid, 12)
	popular, _ := s.Store.Popular(pid, 10)
	recent, _ := s.Store.Recent(pid, 12)
	genres, _ := s.Store.Genres()
	if len(featured) == 0 && len(recent) > 0 {
		featured = recent[:min(3, len(recent))]
	}
	return writeJSON(w, map[string]any{
		"featured": featured, "continue": cont, "popular": popular, "recent": recent,
		"genres": genres, "scan": s.Scanner.Status(),
	})
}

func (s *Server) titles(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	list, total, err := s.Store.Titles(pid, store.Query{
		Kind: q.Get("kind"), Search: q.Get("search"), Genre: q.Get("genre"), Year: queryInt(r, "year", 0),
		Watched: q.Get("watched"), List: q.Get("list"), Sort: q.Get("sort"), Limit: queryInt(r, "limit", 0), Offset: queryInt(r, "offset", 0),
	})
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"items": list, "total": total})
}

func (s *Server) genres(w http.ResponseWriter, r *http.Request) error {
	g, err := s.Store.Genres()
	if err != nil {
		return err
	}
	return writeJSON(w, g)
}

// search finds titles by name and people in the cast.
func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return writeJSON(w, map[string]any{"titles": []any{}, "people": []any{}})
	}
	titles, _, err := s.Store.Titles(pid, store.Query{Search: q, Limit: 40})
	if err != nil {
		return err
	}
	type person struct {
		Name   string        `json:"name"`
		Photo  string        `json:"photo"`
		Titles []store.Title `json:"titles"`
	}
	people := map[string]*person{}
	all, _, _ := s.Store.Titles(pid, store.Query{})
	lq := strings.ToLower(q)
	for _, t := range all {
		var cast []store.CastMember
		table := "media_items"
		if t.Kind == "show" {
			table = "shows"
		}
		var raw string
		_ = s.Store.DB.QueryRow(`SELECT cast_json FROM `+table+` WHERE id=?`, t.ID).Scan(&raw)
		_ = json.Unmarshal([]byte(raw), &cast)
		for _, c := range cast {
			if strings.Contains(strings.ToLower(c.Name), lq) {
				p := people[c.Name]
				if p == nil {
					p = &person{Name: c.Name, Photo: c.Photo}
					people[c.Name] = p
				}
				p.Titles = append(p.Titles, t)
			}
		}
	}
	plist := []*person{}
	for _, p := range people {
		plist = append(plist, p)
	}
	return writeJSON(w, map[string]any{"titles": titles, "people": plist})
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	it, err := s.Store.Item(pid, id)
	if err != nil {
		return err
	}
	if it == nil {
		return notFound("не найдено")
	}
	next, _ := s.Store.NextEpisode(pid, id)
	similar := []store.Title{}
	if len(it.Genres) > 0 {
		cands, _, _ := s.Store.Titles(pid, store.Query{Kind: "movie", Genre: it.Genres[0], Sort: "rating"})
		for _, c := range cands {
			if c.ID != it.ID && len(similar) < 8 {
				similar = append(similar, c)
			}
		}
	}
	return writeJSON(w, map[string]any{"item": it, "next": next, "similar": similar, "compat": s.compat(it), "metaLang": s.Store.Setting("metaLang", "ru-RU")})
}

func (s *Server) show(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	sh, err := s.Store.Show(pid, id)
	if err != nil {
		return err
	}
	if sh == nil {
		return notFound("сериал не найден")
	}
	// "Continue" target: first unwatched episode after the last one in progress.
	var nextUp *store.Item
	for _, season := range sh.Seasons {
		for i := range season.Episodes {
			e := &season.Episodes[i]
			if nextUp == nil && (e.Progress == nil || !e.Progress.Watched) {
				nextUp = e
			}
		}
	}
	return writeJSON(w, map[string]any{"show": sh, "nextUp": nextUp})
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct {
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
		Device   string  `json:"device"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if b.Device == "" {
		b.Device = "pc"
	}
	watched, err := s.Store.SaveProgress(pid, id, b.Position, b.Duration, b.Device)
	if err != nil {
		return err
	}
	s.Bus.Publish("progress", map[string]any{"profileId": pid, "itemId": id, "position": b.Position, "duration": b.Duration, "device": b.Device})
	return writeJSON(w, map[string]bool{"watched": watched})
}

func (s *Server) itemWatched(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct{ Watched bool }
	if err := readJSON(r, &b); err != nil {
		return err
	}
	return s.okOr(w, s.Store.SetWatched(pid, id, b.Watched))
}

func (s *Server) showWatched(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct{ Watched bool }
	if err := readJSON(r, &b); err != nil {
		return err
	}
	return s.okOr(w, s.Store.SetShowWatched(pid, id, b.Watched))
}

func (s *Server) setList(w http.ResponseWriter, r *http.Request) error {
	pid, err := s.requireProfile(r)
	if err != nil {
		return err
	}
	var b struct {
		ItemID int64 `json:"itemId"`
		ShowID int64 `json:"showId"`
		On     bool  `json:"on"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if err := s.Store.SetList(pid, r.PathValue("list"), b.ItemID, b.ShowID, b.On); err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) okOr(w http.ResponseWriter, err error) error {
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

// ---------- manual matching (FR-4) ----------

func (s *Server) matchSearch(w http.ResponseWriter, r *http.Request) error {
	kind := "movie"
	if r.URL.Query().Get("kind") == "show" {
		kind = "tv"
	}
	res, err := s.TMDB.Search(r.Context(), kind, r.URL.Query().Get("q"), queryInt(r, "year", 0))
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, res)
}

func (s *Server) matchItem(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct {
		TmdbID int64 `json:"tmdbId"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if err := s.Scanner.Rematch(r.Context(), "movie", id, b.TmdbID); err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) matchShow(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var b struct {
		TmdbID int64 `json:"tmdbId"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if err := s.Scanner.Rematch(r.Context(), "show", id, b.TmdbID); err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	p := filepath.Join(s.Cfg.DataDir, "images", name)
	if _, err := os.Stat(p); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
	http.ServeFile(w, r, p)
}

func itoa(n int) string { return strconv.Itoa(n) }
