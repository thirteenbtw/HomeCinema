package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/thirteenbtw/homecinema/server/internal/tmdb"
	"github.com/thirteenbtw/homecinema/server/internal/torrent"
)

// Catalog: browse all of TMDB (not just the library) and fetch a title from
// the search sources (JacRed or the user's Torznab), either to download or to watch while it loads.

func catalogErr(err error) error {
	if errors.Is(err, tmdb.ErrNoKey) {
		return httpError{http.StatusPreconditionFailed, "для каталога нужен ключ TMDB — добавьте его в настройках"}
	}
	return httpError{http.StatusBadGateway, err.Error()}
}

func catalogKind(r *http.Request, name string) string {
	if v := r.PathValue(name); v == "tv" || r.URL.Query().Get(name) == "tv" {
		return "tv"
	}
	return "movie"
}

func (s *Server) catalogBrowse(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	page, err := s.TMDB.Browse(r.Context(), tmdb.BrowseQuery{
		Kind: catalogKind(r, "kind"), List: q.Get("list"), Genre: queryInt(r, "genre", 0),
		Year: queryInt(r, "year", 0), Sort: q.Get("sort"), Page: queryInt(r, "page", 1),
	})
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, page)
}

func (s *Server) catalogSearch(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return writeJSON(w, tmdb.CatalogPage{Items: []tmdb.CatalogItem{}, Page: 1})
	}
	page, err := s.TMDB.SearchCatalog(r.Context(), q, queryInt(r, "page", 1))
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, page)
}

func (s *Server) catalogGenres(w http.ResponseWriter, r *http.Request) error {
	g, err := s.TMDB.Genres(r.Context(), catalogKind(r, "kind"))
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, g)
}

type libraryRef struct {
	Kind string `json:"kind"` // movie | show (library routes)
	ID   int64  `json:"id"`
}

func (s *Server) catalogTitle(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	kind := catalogKind(r, "kind")
	t, err := s.TMDB.Title(r.Context(), kind, id)
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, map[string]any{
		"title":    t,
		"library":  s.inLibrary(kind, id),
		"indexers": len(s.Torrents.Indexers()) > 0,
	})
}

// inLibrary finds a title already in the local library by its TMDB id.
func (s *Server) inLibrary(kind string, tmdbID int64) *libraryRef {
	var id int64
	if kind == "tv" {
		if s.Store.DB.QueryRow(`SELECT id FROM shows WHERE tmdb_id=? LIMIT 1`, tmdbID).Scan(&id) == nil {
			return &libraryRef{Kind: "show", ID: id}
		}
		return nil
	}
	if s.Store.DB.QueryRow(`SELECT id FROM media_items WHERE kind='movie' AND tmdb_id=? LIMIT 1`, tmdbID).Scan(&id) == nil {
		return &libraryRef{Kind: "movie", ID: id}
	}
	return nil
}

func (s *Server) catalogTorrents(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if len(s.Torrents.Indexers()) == 0 {
		return badRequest("включите источник поиска (JacRed или свой Torznab) в настройках")
	}
	kind := catalogKind(r, "kind")
	t, err := s.TMDB.Title(r.Context(), kind, id)
	if err != nil {
		return catalogErr(err)
	}
	tq := torrent.TitleQuery{Names: []string{t.Title, t.Orig}, Year: t.Year, Show: kind == "tv", Season: queryInt(r, "season", 0)}
	res, errs, loose := s.Torrents.SearchTitle(r.Context(), tq)
	if errs == nil {
		errs = []string{}
	}
	return writeJSON(w, map[string]any{"results": res, "errors": errs, "loose": loose})
}

// catalogImage proxies and caches TMDB artwork for catalog pages.
func (s *Server) catalogImage(w http.ResponseWriter, r *http.Request) {
	size := r.PathValue("size")
	name, err := s.TMDB.CatalogImage(r.Context(), size, "/"+r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
	http.ServeFile(w, r, filepath.Join(s.TMDB.ImageDir, name))
}

func (s *Server) catalogPerson(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.TMDB.Person(r.Context(), id)
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, p)
}

// catalogPeople lists popular people, or finds people by name (?q=).
func (s *Server) catalogPeople(w http.ResponseWriter, r *http.Request) error {
	people, pages, err := s.TMDB.People(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), queryInt(r, "page", 1))
	if err != nil {
		return catalogErr(err)
	}
	return writeJSON(w, map[string]any{"items": people, "page": queryInt(r, "page", 1), "pages": pages})
}
