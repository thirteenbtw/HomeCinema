// Package api is the HTTP server: REST + SSE for the UI, media streams for the
// PC player and TVs.
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/config"
	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/library"
	"github.com/thirteenbtw/homecinema/server/internal/netutil"
	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/ratings"
	"github.com/thirteenbtw/homecinema/server/internal/renderer"
	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/tmdb"
	"github.com/thirteenbtw/homecinema/server/internal/torrent"
	"github.com/thirteenbtw/homecinema/server/internal/transcode"
)

type Server struct {
	Cfg      config.Config
	Store    *store.Store
	Scanner  *library.Scanner
	TMDB     *tmdb.Client
	Trans    *transcode.Manager
	Prober   *probe.Prober
	Torrents *torrent.Engine
	Cast     *renderer.Manager
	Bus      *events.Bus
	Web      fs.FS
	Version  string
	Port     int

	secret  []byte
	probes  *probeCache
	ratings *ratings.Client
	lb      lbState
	lbHTTP  *http.Client
}

func (s *Server) Handler() http.Handler {
	s.secret = s.loadSecret()
	s.probes = newProbeCache()
	s.ratings = ratings.New()
	s.lbHTTP = &http.Client{Timeout: 20 * time.Second}
	go s.letterboxdLoop()
	mux := http.NewServeMux()
	s.routes(mux)
	return s.middleware(mux)
}

func (s *Server) loadSecret() []byte {
	if v := s.Store.Setting("secret", ""); v != "" {
		if b, err := hex.DecodeString(v); err == nil {
			return b
		}
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	_ = s.Store.SetSetting("secret", hex.EncodeToString(b))
	return b
}

func (s *Server) routes(mux *http.ServeMux) {
	h := func(pattern string, f func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := f(w, r); err != nil {
				var he httpError
				if errors.As(err, &he) {
					writeErr(w, he.code, he.msg)
					return
				}
				log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
				writeErr(w, http.StatusInternalServerError, err.Error())
			}
		})
	}
	// session & profiles
	h("GET /api/state", s.state)
	h("POST /api/auth/login", s.login)
	h("POST /api/auth/logout", s.logout)
	h("GET /api/profiles", s.listProfiles)
	h("POST /api/profiles", s.createProfile)
	h("PATCH /api/profiles/{id}", s.updateProfile)
	h("DELETE /api/profiles/{id}", s.deleteProfile)
	h("POST /api/profiles/{id}/select", s.selectProfile)

	// library
	h("GET /api/home", s.home)
	h("GET /api/titles", s.titles)
	h("GET /api/genres", s.genres)
	h("GET /api/search", s.search)
	h("GET /api/items/{id}", s.item)
	h("GET /api/shows/{id}", s.show)
	h("POST /api/items/{id}/progress", s.progress)
	h("POST /api/items/{id}/watched", s.itemWatched)
	h("POST /api/shows/{id}/watched", s.showWatched)
	h("POST /api/lists/{list}", s.setList)
	h("GET /api/match/search", s.matchSearch)
	h("POST /api/items/{id}/match", s.matchItem)
	h("POST /api/shows/{id}/match", s.matchShow)
	mux.HandleFunc("GET /api/images/{name}", s.image)

	// likes, watch later, watched (catalog titles) and Letterboxd
	h("GET /api/marks", s.listMarks)
	h("GET /api/marks/keys", s.markKeys)
	h("POST /api/marks", s.setMark)
	h("GET /api/letterboxd", s.letterboxdState)
	h("PUT /api/letterboxd", s.putLetterboxd)
	h("POST /api/letterboxd/sync", s.syncLetterboxd)
	h("POST /api/letterboxd/import", s.importLetterboxd)
	h("DELETE /api/letterboxd/marks", s.clearLetterboxd)
	h("GET /api/letterboxd/export/{name}", s.exportLetterboxd)

	// catalog (all of TMDB)
	h("GET /api/catalog", s.catalogBrowse)
	h("GET /api/catalog/search", s.catalogSearch)
	h("GET /api/catalog/genres", s.catalogGenres)
	h("GET /api/catalog/title/{kind}/{id}", s.catalogTitle)
	h("GET /api/catalog/title/{kind}/{id}/torrents", s.catalogTorrents)
	h("GET /api/catalog/title/{kind}/{id}/ratings", s.catalogRatings)
	h("GET /api/catalog/person/{id}", s.catalogPerson)
	h("GET /api/catalog/people", s.catalogPeople)
	mux.HandleFunc("GET /api/catalog-img/{size}/{name}", s.catalogImage)

	// playback
	h("GET /api/play/{fileId}", s.playInfo)
	h("POST /api/files/{id}/external", s.openExternal)
	mux.HandleFunc("GET /api/files/{id}/direct", s.direct)
	mux.HandleFunc("GET /api/files/{id}/subs/{n}", s.subtitle)
	mux.HandleFunc("GET /api/hls/{session}/{file}", s.hls)
	h("DELETE /api/hls/{session}", s.stopHLS)
	// token-authenticated copies for TVs (no cookies there)
	mux.HandleFunc("GET /api/t/{token}/files/{id}/direct", s.withToken(s.direct))
	mux.HandleFunc("GET /api/t/{token}/hls/{session}/{file}", s.withToken(s.hls))
	mux.HandleFunc("GET /api/t/{token}/torrents/{hash}/files/{idx}", s.withToken(s.torrentFile))
	mux.HandleFunc("GET /api/t/{token}/files/{id}/subs/{n}", s.withToken(s.subtitle))

	// torrents
	h("GET /api/torrents", s.listTorrents)
	h("POST /api/torrents", s.addTorrent)
	h("GET /api/torrents/search", s.searchTorrents)
	h("GET /api/torrents/{hash}", s.getTorrent)
	h("PATCH /api/torrents/{hash}", s.patchTorrent)
	h("DELETE /api/torrents/{hash}", s.deleteTorrent)
	h("GET /api/torrents/{hash}/stream", s.torrentStream)
	h("GET /api/torrents/{hash}/play", s.torrentPlay)
	mux.HandleFunc("GET /api/torrents/{hash}/files/{idx}", s.torrentFile)
	h("GET /api/indexers", s.getIndexers)
	h("PUT /api/indexers", s.putIndexers)
	h("GET /api/torrent-settings", s.getTorrentSettings)
	h("PUT /api/torrent-settings", s.putTorrentSettings)
	h("GET /api/interfaces", s.interfaces)

	// devices & cast
	h("GET /api/devices", s.devices)
	h("POST /api/devices/scan", s.scanDevices)
	h("POST /api/devices", s.addDevice)
	h("DELETE /api/devices/{id}", s.removeDevice)
	h("GET /api/cast", s.castStatus)
	h("POST /api/cast/{id}/load", s.castLoad)
	h("POST /api/cast/{id}/{action}", s.castControl)

	// settings & maintenance
	h("GET /api/settings", s.getSettings)
	h("PUT /api/settings", s.putSettings)
	h("GET /api/libraries", s.libraries)
	h("POST /api/libraries", s.addLibrary)
	h("DELETE /api/libraries/{id}", s.removeLibrary)
	h("POST /api/libraries/scan", s.rescan)
	h("GET /api/fs", s.browseFS)
	h("GET /api/diagnostics", s.diagnostics)
	h("GET /api/backup", s.backup)
	h("POST /api/restore", s.restore)
	h("GET /api/export/watched.csv", s.exportWatched)
	h("GET /api/logs", s.logs)
	mux.HandleFunc("GET /api/events", s.events)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	mux.Handle("/", s.spa())
}

// ---------- middleware: LAN only, optional password, profile ----------

type ctxKey int

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := netutil.RemoteIP(r)
		if ip == nil || !netutil.IsLAN(ip) {
			http.Error(w, "HomeCinema доступен только из локальной сети", http.StatusForbidden)
			return
		}
		if s.Cfg.Dev {
			if o := r.Header.Get("Origin"); o != "" {
				w.Header().Set("Access-Control-Allow-Origin", o)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE")
				if r.Method == http.MethodOptions {
					return
				}
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") && !strings.HasPrefix(p, "/api/t/") && p != "/api/state" && p != "/api/auth/login" && !strings.HasPrefix(p, "/api/images/") {
			if !s.authorized(r) {
				writeErr(w, http.StatusUnauthorized, "нужен пароль")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// authorized: the password (FR-20) is optional; the desktop window on the same
// PC (loopback) never needs it.
func (s *Server) authorized(r *http.Request) bool {
	if s.Store.Setting("password", "") == "" || netutil.IsLoopback(r) {
		return true
	}
	c, err := r.Cookie("hc_auth")
	return err == nil && s.verify("auth", c.Value)
}

func (s *Server) sign(kind, value string, ttl time.Duration) string {
	exp := strconv.FormatInt(time.Now().Add(ttl).Unix(), 36)
	payload := kind + "|" + value + "|" + exp
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(value+"|"+exp)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

// verify checks a token and returns whether it is valid for kind.
func (s *Server) verify(kind, token string) bool {
	_, ok := s.tokenValue(kind, token)
	return ok
}

func (s *Server) tokenValue(kind, token string) (string, bool) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	value, expS, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", false
	}
	exp, err := strconv.ParseInt(expS, 36, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(kind + "|" + value + "|" + expS))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", false
	}
	return value, true
}

// streamToken grants a TV access to one resource for a few hours. It names the
// resource, never a file path, and stops working after it expires.
func (s *Server) streamToken(resource string) string {
	return s.sign("stream", resource, 6*time.Hour)
}

func (s *Server) withToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, ok := s.tokenValue("stream", r.PathValue("token"))
		if !ok {
			http.Error(w, "ссылка устарела", http.StatusForbidden)
			return
		}
		// The token is bound to the file / torrent / HLS session it was issued for.
		want := ""
		switch {
		case r.PathValue("session") != "":
			want = "hls:" + r.PathValue("session")
		case r.PathValue("hash") != "":
			want = "torrent:" + strings.ToLower(r.PathValue("hash"))
		default:
			want = "file:" + r.PathValue("id")
		}
		if res != want {
			http.Error(w, "токен не подходит", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// profileID returns the active profile from the signed cookie, or the first
// profile without a PIN.
func (s *Server) profileID(r *http.Request) int64 {
	if c, err := r.Cookie("hc_profile"); err == nil {
		if v, ok := s.tokenValue("profile", c.Value); ok {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				return id
			}
		}
	}
	profiles, _ := s.Store.Profiles()
	for _, p := range profiles {
		if !p.HasPIN {
			return p.ID
		}
	}
	return 0
}

func (s *Server) requireProfile(r *http.Request) (int64, error) {
	if id := s.profileID(r); id > 0 {
		return id, nil
	}
	return 0, httpError{http.StatusUnauthorized, "выберите профиль"}
}

// ---------- helpers ----------

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return httpError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}
func notFound(msg string) error { return httpError{http.StatusNotFound, msg} }

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	return json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20)).Decode(v); err != nil {
		return badRequest("неверный JSON: %v", err)
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, badRequest("неверный id")
	}
	return id, nil
}

func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
}

func queryFloat(r *http.Request, name string) float64 {
	v, _ := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	return v
}

// ---------- SPA ----------

func (s *Server) spa() http.Handler {
	files := http.FileServer(http.FS(s.Web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := s.Web.Open(p); err == nil {
				f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// client-side routes fall back to index.html
		b, err := fs.ReadFile(s.Web, "index.html")
		if err != nil {
			http.Error(w, "UI не собран: выполните npm run build в web/", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	})
}

// ---------- SSE ----------

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.Bus.Subscribe()
	defer cancel()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}
