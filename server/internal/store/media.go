package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Track struct {
	Index    int    `json:"index"` // stream index inside the file, or -1 for external
	Codec    string `json:"codec"`
	Lang     string `json:"lang"`
	Title    string `json:"title"`
	Channels int    `json:"channels,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
	External string `json:"external,omitempty"` // path of an external subtitle file
}

type File struct {
	ID         int64   `json:"id"`
	ItemID     int64   `json:"itemId"`
	Path       string  `json:"-"`
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	Mtime      int64   `json:"-"`
	Container  string  `json:"container"`
	Duration   float64 `json:"duration"`
	VideoCodec string  `json:"videoCodec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	BitDepth   int     `json:"bitDepth"`
	HDR        string  `json:"hdr"`
	Audio      []Track `json:"audio"`
	Subs       []Track `json:"subs"`
	Probed     bool    `json:"-"`
}

type CastMember struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Photo   string `json:"photo"`
	Profile string `json:"-"`
}

type Progress struct {
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Watched   bool    `json:"watched"`
	Device    string  `json:"device"`
	UpdatedAt int64   `json:"updatedAt"`
}

type Item struct {
	ID        int64        `json:"id"`
	LibraryID int64        `json:"libraryId"`
	Kind      string       `json:"kind"`
	Title     string       `json:"title"`
	OrigTitle string       `json:"origTitle,omitempty"`
	Year      int          `json:"year"`
	ShowID    int64        `json:"showId,omitempty"`
	ShowTitle string       `json:"showTitle,omitempty"`
	Season    int          `json:"season,omitempty"`
	Episode   int          `json:"episode,omitempty"`
	EpTitle   string       `json:"epTitle,omitempty"`
	TmdbID    int64        `json:"tmdbId"`
	Overview  string       `json:"overview"`
	Poster    string       `json:"poster"`
	Backdrop  string       `json:"backdrop"`
	Genres    []string     `json:"genres"`
	Rating    float64      `json:"rating"`
	Runtime   int          `json:"runtime"`
	Cert      string       `json:"cert"`
	Cast      []CastMember `json:"cast,omitempty"`
	Matched   int          `json:"matched"`
	AddedAt   int64        `json:"addedAt"`
	Source    string       `json:"source"`
	Quality   string       `json:"quality"`
	Progress  *Progress    `json:"progress,omitempty"`
	Favorite  bool         `json:"favorite"`
	Later     bool         `json:"later"`
	Files     []File       `json:"files,omitempty"`
}

type Show struct {
	ID        int64        `json:"id"`
	Title     string       `json:"title"`
	Year      int          `json:"year"`
	TmdbID    int64        `json:"tmdbId"`
	Overview  string       `json:"overview"`
	Poster    string       `json:"poster"`
	Backdrop  string       `json:"backdrop"`
	Genres    []string     `json:"genres"`
	Rating    float64      `json:"rating"`
	Cast      []CastMember `json:"cast,omitempty"`
	Matched   int          `json:"matched"`
	Favorite  bool         `json:"favorite"`
	Later     bool         `json:"later"`
	Episodes  int          `json:"episodes"`
	Unwatched int          `json:"unwatched"`
	AddedAt   int64        `json:"addedAt"`
	Seasons   []Season     `json:"seasons,omitempty"`
}

type Season struct {
	Number   int    `json:"number"`
	Episodes []Item `json:"episodes"`
}

// ---------- files / items written by the scanner ----------

func (s *Store) FileByPath(path string) (*File, error) {
	f := &File{}
	var audio, subs string
	err := s.DB.QueryRow(`SELECT id, item_id, path, size, mtime, container, duration, video_codec, width, height, bit_depth, hdr, audio_json, subs_json, probed FROM files WHERE path=?`, path).
		Scan(&f.ID, &f.ItemID, &f.Path, &f.Size, &f.Mtime, &f.Container, &f.Duration, &f.VideoCodec, &f.Width, &f.Height, &f.BitDepth, &f.HDR, &audio, &subs, &f.Probed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(audio), &f.Audio)
	_ = json.Unmarshal([]byte(subs), &f.Subs)
	return f, nil
}

func (s *Store) AllFilePaths(libraryID int64) (map[string]int64, error) {
	rows, err := s.DB.Query(`SELECT f.path, f.id FROM files f JOIN media_items m ON m.id=f.item_id WHERE m.library_id=?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var p string
		var id int64
		if rows.Scan(&p, &id) == nil {
			out[p] = id
		}
	}
	return out, rows.Err()
}

// UpsertShow finds or creates a show by parsed title/year and returns its id.
func (s *Store) UpsertShow(title string, year int) (int64, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM shows WHERE lower(title)=lower(?) AND (year=? OR ?=0 OR year=0)`, title, year, year).Scan(&id)
	if err == nil {
		return id, nil
	}
	res, err := s.Exec(`INSERT INTO shows(title, year) VALUES(?,?)`, title, year)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type NewItem struct {
	LibraryID int64
	Kind      string
	Title     string
	Year      int
	ShowID    int64
	Season    int
	Episode   int
	Source    string
	File      File
}

func (s *Store) InsertItem(n NewItem) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var showID any
	if n.ShowID > 0 {
		showID = n.ShowID
	}
	res, err := tx.Exec(`INSERT INTO media_items(library_id, kind, title, year, show_id, season, episode, added_at, source) VALUES(?,?,?,?,?,?,?,?,?)`,
		n.LibraryID, n.Kind, n.Title, n.Year, showID, n.Season, n.Episode, now(), n.Source)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	f := n.File
	_, err = tx.Exec(`INSERT INTO files(item_id, path, size, mtime, container, duration, video_codec, width, height, bit_depth, hdr, audio_json, subs_json, probed) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, f.Path, f.Size, f.Mtime, f.Container, f.Duration, f.VideoCodec, f.Width, f.Height, f.BitDepth, f.HDR, toJSON(nz(f.Audio)), toJSON(nz(f.Subs)), f.Probed)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func nz(t []Track) []Track {
	if t == nil {
		return []Track{}
	}
	return t
}

func (s *Store) UpdateFileProbe(f File) error {
	_, err := s.Exec(`UPDATE files SET size=?, mtime=?, container=?, duration=?, video_codec=?, width=?, height=?, bit_depth=?, hdr=?, audio_json=?, subs_json=?, probed=1 WHERE id=?`,
		f.Size, f.Mtime, f.Container, f.Duration, f.VideoCodec, f.Width, f.Height, f.BitDepth, f.HDR, toJSON(nz(f.Audio)), toJSON(nz(f.Subs)), f.ID)
	return err
}

func (s *Store) DeleteFile(fileID int64) error {
	var itemID int64
	if err := s.DB.QueryRow(`SELECT item_id FROM files WHERE id=?`, fileID).Scan(&itemID); err != nil {
		return err
	}
	if _, err := s.Exec(`DELETE FROM files WHERE id=?`, fileID); err != nil {
		return err
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM files WHERE item_id=?`, itemID).Scan(&n)
	if n == 0 {
		_, _ = s.Exec(`DELETE FROM media_items WHERE id=?`, itemID)
		s.PruneShows()
	}
	return nil
}

func (s *Store) PruneShows() {
	_, _ = s.Exec(`DELETE FROM shows WHERE id NOT IN (SELECT DISTINCT show_id FROM media_items WHERE show_id IS NOT NULL)`)
}

// ---------- metadata ----------

type Meta struct {
	TmdbID    int64
	Title     string
	OrigTitle string
	Year      int
	Overview  string
	Poster    string
	Backdrop  string
	Genres    []string
	Rating    float64
	Runtime   int
	Cert      string
	Cast      []CastMember
	EpTitle   string
	Matched   int
}

func (s *Store) ApplyItemMeta(id int64, m Meta) error {
	_, err := s.Exec(`UPDATE media_items SET tmdb_id=?, title=CASE WHEN ?<>'' THEN ? ELSE title END, orig_title=?, year=CASE WHEN ?>0 THEN ? ELSE year END,
		overview=?, poster=?, backdrop=?, genres=?, rating=?, runtime=?, cert=?, cast_json=?, ep_title=?, matched=? WHERE id=?`,
		m.TmdbID, m.Title, m.Title, m.OrigTitle, m.Year, m.Year, m.Overview, m.Poster, m.Backdrop, toJSON(nzs(m.Genres)), m.Rating, m.Runtime, m.Cert, toJSON(nzc(m.Cast)), m.EpTitle, m.Matched, id)
	return err
}

func (s *Store) ApplyShowMeta(id int64, m Meta) error {
	_, err := s.Exec(`UPDATE shows SET tmdb_id=?, title=CASE WHEN ?<>'' THEN ? ELSE title END, year=CASE WHEN ?>0 THEN ? ELSE year END,
		overview=?, poster=?, backdrop=?, genres=?, rating=?, cast_json=?, matched=? WHERE id=?`,
		m.TmdbID, m.Title, m.Title, m.Year, m.Year, m.Overview, m.Poster, m.Backdrop, toJSON(nzs(m.Genres)), m.Rating, toJSON(nzc(m.Cast)), m.Matched, id)
	return err
}

func (s *Store) SetItemMatched(id int64, matched int) error {
	_, err := s.Exec(`UPDATE media_items SET matched=? WHERE id=?`, matched, id)
	return err
}

func (s *Store) SetShowMatched(id int64, matched int) error {
	_, err := s.Exec(`UPDATE shows SET matched=? WHERE id=?`, matched, id)
	return err
}

func nzs(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func nzc(v []CastMember) []CastMember {
	if v == nil {
		return []CastMember{}
	}
	return v
}

type Pending struct {
	ID       int64
	Kind     string // movie | show | episode
	Title    string
	Year     int
	ShowTmdb int64
	Season   int
	Episode  int
}

// PendingMetadata returns movies and shows that still need a TMDB lookup, and
// episodes whose show is matched but the episode itself is not.
func (s *Store) PendingMetadata(limit int) ([]Pending, error) {
	var out []Pending
	rows, err := s.DB.Query(`SELECT id, 'movie', title, year, 0, 0, 0 FROM media_items WHERE kind='movie' AND matched=0
		UNION ALL SELECT id, 'show', title, year, 0, 0, 0 FROM shows WHERE matched=0
		UNION ALL SELECT m.id, 'episode', s.title, 0, s.tmdb_id, m.season, m.episode FROM media_items m JOIN shows s ON s.id=m.show_id WHERE m.kind='episode' AND m.matched=0 AND s.matched IN (1,3)
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.ID, &p.Kind, &p.Title, &p.Year, &p.ShowTmdb, &p.Season, &p.Episode); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResetUnmatched queues items that previously failed (e.g. no API key yet) for another try.
// ResetAllMatches queues every title for matching again, e.g. after the
// metadata provider changed and the stored ids belong to the old one.
func (s *Store) ResetAllMatches() {
	_, _ = s.Exec(`UPDATE media_items SET matched=0`)
	_, _ = s.Exec(`UPDATE shows SET matched=0`)
}

func (s *Store) ResetUnmatched() {
	_, _ = s.Exec(`UPDATE media_items SET matched=0 WHERE matched=2`)
	_, _ = s.Exec(`UPDATE shows SET matched=0 WHERE matched=2`)
}

// ---------- reads for the UI ----------

const itemCols = `m.id, IFNULL(m.library_id,0), m.kind, m.title, m.orig_title, m.year, IFNULL(m.show_id,0), IFNULL(s.title,''), m.season, m.episode, m.ep_title,
	m.tmdb_id, m.overview, m.poster, m.backdrop, m.genres, m.rating, m.runtime, m.cert, m.cast_json, m.matched, m.added_at, m.source,
	IFNULL(f.width,0), IFNULL(f.hdr,''), IFNULL(f.duration,0),
	p.position, p.duration, p.watched, p.device, p.updated_at,
	EXISTS(SELECT 1 FROM lists l WHERE l.profile_id=? AND l.list='favorites' AND l.item_id=m.id),
	EXISTS(SELECT 1 FROM lists l WHERE l.profile_id=? AND l.list='later' AND l.item_id=m.id)`

const itemFrom = ` FROM media_items m
	LEFT JOIN shows s ON s.id=m.show_id
	LEFT JOIN files f ON f.id=(SELECT id FROM files WHERE item_id=m.id ORDER BY size DESC LIMIT 1)
	LEFT JOIN watch_progress p ON p.item_id=m.id AND p.profile_id=? `

func scanItem(rows interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var genres, cast string
	var width int
	var hdr string
	var fdur float64
	var pos, dur sql.NullFloat64
	var watched sql.NullBool
	var device sql.NullString
	var upd sql.NullInt64
	err := rows.Scan(&it.ID, &it.LibraryID, &it.Kind, &it.Title, &it.OrigTitle, &it.Year, &it.ShowID, &it.ShowTitle, &it.Season, &it.Episode, &it.EpTitle,
		&it.TmdbID, &it.Overview, &it.Poster, &it.Backdrop, &genres, &it.Rating, &it.Runtime, &it.Cert, &cast, &it.Matched, &it.AddedAt, &it.Source,
		&width, &hdr, &fdur, &pos, &dur, &watched, &device, &upd, &it.Favorite, &it.Later)
	if err != nil {
		return it, err
	}
	it.Genres = jsonStrings(genres)
	_ = json.Unmarshal([]byte(cast), &it.Cast)
	it.Quality = QualityLabel(width, hdr)
	if it.Runtime == 0 && fdur > 0 {
		it.Runtime = int(fdur / 60)
	}
	if pos.Valid {
		it.Progress = &Progress{Position: pos.Float64, Duration: dur.Float64, Watched: watched.Bool, Device: device.String, UpdatedAt: upd.Int64}
	}
	return it, nil
}

func QualityLabel(width int, hdr string) string {
	q := ""
	switch {
	case width >= 3200:
		q = "4K"
	case width >= 1800:
		q = "1080p"
	case width >= 1200:
		q = "720p"
	case width > 0:
		q = "SD"
	}
	if hdr != "" && q != "" {
		q += " " + hdr
	}
	return q
}

func (s *Store) queryItems(profile int64, where string, order string, limit int, args ...any) ([]Item, error) {
	q := `SELECT ` + itemCols + itemFrom + ` WHERE ` + where + ` ORDER BY ` + order
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	all := append([]any{profile, profile, profile}, args...)
	rows, err := s.DB.Query(q, all...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		it.Cast = nil
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// Lists are small (one page, one season); the UI needs file ids to play.
	for i := range out {
		if out[i].Files, err = s.Files(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) Item(profile, id int64) (*Item, error) {
	q := `SELECT ` + itemCols + itemFrom + ` WHERE m.id=?`
	it, err := scanItem(s.DB.QueryRow(q, profile, profile, profile, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	it.Files, err = s.Files(id)
	return &it, err
}

func (s *Store) Files(itemID int64) ([]File, error) {
	rows, err := s.DB.Query(`SELECT id, item_id, path, size, mtime, container, duration, video_codec, width, height, bit_depth, hdr, audio_json, subs_json, probed FROM files WHERE item_id=? ORDER BY size DESC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		var f File
		var audio, subs string
		if err := rows.Scan(&f.ID, &f.ItemID, &f.Path, &f.Size, &f.Mtime, &f.Container, &f.Duration, &f.VideoCodec, &f.Width, &f.Height, &f.BitDepth, &f.HDR, &audio, &subs, &f.Probed); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(audio), &f.Audio)
		_ = json.Unmarshal([]byte(subs), &f.Subs)
		f.Name = baseName(f.Path)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) FileByID(id int64) (*File, error) {
	var itemID int64
	if err := s.DB.QueryRow(`SELECT item_id FROM files WHERE id=?`, id).Scan(&itemID); err != nil {
		return nil, err
	}
	files, err := s.Files(itemID)
	if err != nil {
		return nil, err
	}
	for i := range files {
		if files[i].ID == id {
			return &files[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func baseName(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	return p[i+1:]
}

type Query struct {
	Kind    string // movie | show | "" (both)
	Search  string
	Genre   string
	Year    int
	Watched string // yes | no | ""
	List    string // favorites | later
	Sort    string // added | title | year | rating
	Limit   int
	Offset  int
}

// Title is one entry in a library grid: a movie or a whole show.
type Title struct {
	Kind     string   `json:"kind"` // movie | show
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Year     int      `json:"year"`
	Poster   string   `json:"poster"`
	Backdrop string   `json:"backdrop"`
	Genres   []string `json:"genres"`
	Rating   float64  `json:"rating"`
	Quality  string   `json:"quality"`
	AddedAt  int64    `json:"addedAt"`
	Watched  bool     `json:"watched"`
	Progress float64  `json:"progress"` // 0..1
	Favorite bool     `json:"favorite"`
	Later    bool     `json:"later"`
	Episodes int      `json:"episodes,omitempty"`
	Overview string   `json:"overview"`
}

// Titles lists movies and shows together, filtered and sorted in Go: libraries
// of a few thousand titles stay well under the 1 s budget from the spec.
func (s *Store) Titles(profile int64, q Query) ([]Title, int, error) {
	var out []Title
	if q.Kind != "show" {
		movies, err := s.queryItems(profile, `m.kind='movie'`, `m.added_at DESC`, 0)
		if err != nil {
			return nil, 0, err
		}
		for _, m := range movies {
			t := Title{Kind: "movie", ID: m.ID, Title: m.Title, Year: m.Year, Poster: m.Poster, Backdrop: m.Backdrop, Genres: m.Genres, Rating: m.Rating,
				Quality: m.Quality, AddedAt: m.AddedAt, Favorite: m.Favorite, Later: m.Later, Overview: m.Overview}
			if m.Progress != nil {
				t.Watched = m.Progress.Watched
				if m.Progress.Duration > 0 {
					t.Progress = m.Progress.Position / m.Progress.Duration
				}
			}
			out = append(out, t)
		}
	}
	if q.Kind != "movie" {
		shows, err := s.Shows(profile)
		if err != nil {
			return nil, 0, err
		}
		for _, sh := range shows {
			out = append(out, Title{Kind: "show", ID: sh.ID, Title: sh.Title, Year: sh.Year, Poster: sh.Poster, Backdrop: sh.Backdrop, Genres: sh.Genres,
				Rating: sh.Rating, AddedAt: sh.AddedAt, Watched: sh.Unwatched == 0, Favorite: sh.Favorite, Later: sh.Later, Episodes: sh.Episodes, Overview: sh.Overview})
		}
	}
	search := strings.ToLower(strings.TrimSpace(q.Search))
	filtered := out[:0]
	for _, t := range out {
		if search != "" && !strings.Contains(strings.ToLower(t.Title), search) {
			continue
		}
		if q.Genre != "" && !contains(t.Genres, q.Genre) {
			continue
		}
		if q.Year > 0 && t.Year != q.Year {
			continue
		}
		if q.Watched == "yes" && !t.Watched || q.Watched == "no" && t.Watched {
			continue
		}
		if q.List == "favorites" && !t.Favorite || q.List == "later" && !t.Later {
			continue
		}
		filtered = append(filtered, t)
	}
	switch q.Sort {
	case "title":
		sort.SliceStable(filtered, func(i, j int) bool { return strings.ToLower(filtered[i].Title) < strings.ToLower(filtered[j].Title) })
	case "year":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Year > filtered[j].Year })
	case "rating":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Rating > filtered[j].Rating })
	default:
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].AddedAt > filtered[j].AddedAt })
	}
	total := len(filtered)
	if q.Offset > 0 && q.Offset < len(filtered) {
		filtered = filtered[q.Offset:]
	} else if q.Offset >= len(filtered) && q.Offset > 0 {
		filtered = nil
	}
	if q.Limit > 0 && len(filtered) > q.Limit {
		filtered = filtered[:q.Limit]
	}
	if filtered == nil {
		filtered = []Title{}
	}
	return filtered, total, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func (s *Store) Shows(profile int64) ([]Show, error) {
	rows, err := s.DB.Query(`SELECT s.id, s.title, s.year, s.tmdb_id, s.overview, s.poster, s.backdrop, s.genres, s.rating, s.matched,
		(SELECT COUNT(*) FROM media_items WHERE show_id=s.id),
		(SELECT COUNT(*) FROM media_items m WHERE m.show_id=s.id AND NOT EXISTS(SELECT 1 FROM watch_progress p WHERE p.item_id=m.id AND p.profile_id=? AND p.watched=1)),
		IFNULL((SELECT MAX(added_at) FROM media_items WHERE show_id=s.id),0),
		EXISTS(SELECT 1 FROM lists l WHERE l.profile_id=? AND l.list='favorites' AND l.show_id=s.id),
		EXISTS(SELECT 1 FROM lists l WHERE l.profile_id=? AND l.list='later' AND l.show_id=s.id)
		FROM shows s ORDER BY s.title`, profile, profile, profile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Show{}
	for rows.Next() {
		var sh Show
		var genres string
		if err := rows.Scan(&sh.ID, &sh.Title, &sh.Year, &sh.TmdbID, &sh.Overview, &sh.Poster, &sh.Backdrop, &genres, &sh.Rating, &sh.Matched,
			&sh.Episodes, &sh.Unwatched, &sh.AddedAt, &sh.Favorite, &sh.Later); err != nil {
			return nil, err
		}
		sh.Genres = jsonStrings(genres)
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (s *Store) Show(profile, id int64) (*Show, error) {
	shows, err := s.Shows(profile)
	if err != nil {
		return nil, err
	}
	var sh *Show
	for i := range shows {
		if shows[i].ID == id {
			sh = &shows[i]
		}
	}
	if sh == nil {
		return nil, nil
	}
	var cast string
	_ = s.DB.QueryRow(`SELECT cast_json FROM shows WHERE id=?`, id).Scan(&cast)
	_ = json.Unmarshal([]byte(cast), &sh.Cast)
	eps, err := s.queryItems(profile, `m.show_id=?`, `m.season, m.episode`, 0, id)
	if err != nil {
		return nil, err
	}
	for _, e := range eps {
		if len(sh.Seasons) == 0 || sh.Seasons[len(sh.Seasons)-1].Number != e.Season {
			sh.Seasons = append(sh.Seasons, Season{Number: e.Season})
		}
		last := &sh.Seasons[len(sh.Seasons)-1]
		last.Episodes = append(last.Episodes, e)
	}
	return sh, nil
}

// Continue returns started-but-unfinished items, most recent first.
func (s *Store) Continue(profile int64, limit int) ([]Item, error) {
	return s.queryItems(profile, `p.position > 30 AND p.watched=0`, `p.updated_at DESC`, limit)
}

func (s *Store) Recent(profile int64, limit int) ([]Item, error) {
	items, err := s.queryItems(profile, `1=1`, `m.added_at DESC, m.season DESC, m.episode DESC`, limit*4)
	if err != nil {
		return nil, err
	}
	// Collapse a freshly added season to one card per show.
	seen := map[int64]bool{}
	out := []Item{}
	for _, it := range items {
		if it.ShowID > 0 {
			if seen[it.ShowID] {
				continue
			}
			seen[it.ShowID] = true
		}
		out = append(out, it)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Popular ranks titles by plays across all profiles of the household.
func (s *Store) Popular(profile int64, limit int) ([]Item, error) {
	return s.queryItems(profile, `m.kind='movie' OR m.id IN (SELECT MIN(id) FROM media_items WHERE kind='episode' GROUP BY show_id)`,
		`(SELECT IFNULL(SUM(plays),0) FROM watch_progress w WHERE w.item_id=m.id OR w.item_id IN (SELECT id FROM media_items x WHERE x.show_id=m.show_id AND m.show_id IS NOT NULL)) DESC, m.rating DESC`, limit)
}

func (s *Store) Featured(profile int64, limit int) ([]Item, error) {
	return s.queryItems(profile, `m.kind='movie' AND m.backdrop<>''`, `m.added_at DESC`, limit)
}

func (s *Store) Genres() ([]string, error) {
	rows, err := s.DB.Query(`SELECT genres FROM media_items WHERE kind='movie' UNION ALL SELECT genres FROM shows`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := map[string]int{}
	for rows.Next() {
		var g string
		if rows.Scan(&g) == nil {
			for _, x := range jsonStrings(g) {
				count[x]++
			}
		}
	}
	out := make([]string, 0, len(count))
	for g := range count {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return count[out[i]] > count[out[j]] })
	return out, nil
}

func (s *Store) Counts() (movies, shows int) {
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM media_items WHERE kind='movie'`).Scan(&movies)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM shows`).Scan(&shows)
	return
}

// NextEpisode returns the episode after the given one in the same show.
func (s *Store) NextEpisode(profile, itemID int64) (*Item, error) {
	var showID int64
	var season, ep int
	if err := s.DB.QueryRow(`SELECT IFNULL(show_id,0), season, episode FROM media_items WHERE id=?`, itemID).Scan(&showID, &season, &ep); err != nil || showID == 0 {
		return nil, err
	}
	items, err := s.queryItems(profile, `m.show_id=? AND (m.season>? OR (m.season=? AND m.episode>?))`, `m.season, m.episode`, 1, showID, season, season, ep)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}

// ---------- progress & lists ----------

// WatchedThreshold: an item counts as watched once 90% has been played (FR-10).
const WatchedThreshold = 0.9

func (s *Store) SaveProgress(profile, item int64, position, duration float64, device string) (bool, error) {
	watched := duration > 0 && position/duration >= WatchedThreshold
	_, err := s.Exec(`INSERT INTO watch_progress(profile_id, item_id, position, duration, watched, device, plays, updated_at) VALUES(?,?,?,?,?,?,1,?)
		ON CONFLICT(profile_id, item_id) DO UPDATE SET position=excluded.position, duration=CASE WHEN excluded.duration>0 THEN excluded.duration ELSE duration END,
		watched=CASE WHEN excluded.watched=1 THEN 1 ELSE watched END, device=excluded.device, updated_at=excluded.updated_at,
		plays=plays + CASE WHEN watched=1 AND excluded.watched=0 AND excluded.position<60 THEN 1 ELSE 0 END`,
		profile, item, position, duration, watched, device, now())
	if watched {
		// once finished, the item leaves "Continue watching"
		_, _ = s.Exec(`UPDATE watch_progress SET position=0 WHERE profile_id=? AND item_id=? AND watched=1 AND position/NULLIF(duration,0) >= 0.97`, profile, item)
	}
	return watched, err
}

func (s *Store) SetWatched(profile, item int64, watched bool) error {
	_, err := s.Exec(`INSERT INTO watch_progress(profile_id, item_id, position, watched, updated_at) VALUES(?,?,0,?,?)
		ON CONFLICT(profile_id, item_id) DO UPDATE SET watched=excluded.watched, position=0, updated_at=excluded.updated_at`, profile, item, watched, now())
	return err
}

func (s *Store) SetShowWatched(profile, show int64, watched bool) error {
	rows, err := s.DB.Query(`SELECT id FROM media_items WHERE show_id=?`, show)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if err := s.SetWatched(profile, id, watched); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Progress(profile, item int64) (*Progress, error) {
	var p Progress
	err := s.DB.QueryRow(`SELECT position, duration, watched, device, updated_at FROM watch_progress WHERE profile_id=? AND item_id=?`, profile, item).
		Scan(&p.Position, &p.Duration, &p.Watched, &p.Device, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (s *Store) SetList(profile int64, list string, itemID, showID int64, on bool) error {
	if list != "favorites" && list != "later" {
		return fmt.Errorf("unknown list %q", list)
	}
	if on {
		_, err := s.Exec(`INSERT OR IGNORE INTO lists(profile_id, list, item_id, show_id, added_at) VALUES(?,?,?,?,?)`, profile, list, itemID, showID, now())
		return err
	}
	_, err := s.Exec(`DELETE FROM lists WHERE profile_id=? AND list=? AND item_id=? AND show_id=?`, profile, list, itemID, showID)
	return err
}

type WatchedRow struct {
	Profile string
	Title   string
	Year    int
	Season  int
	Episode int
	Date    int64
}

func (s *Store) WatchedExport() ([]WatchedRow, error) {
	rows, err := s.DB.Query(`SELECT pr.name, IFNULL(sh.title, m.title), m.year, m.season, m.episode, p.updated_at FROM watch_progress p
		JOIN profiles pr ON pr.id=p.profile_id JOIN media_items m ON m.id=p.item_id LEFT JOIN shows sh ON sh.id=m.show_id
		WHERE p.watched=1 ORDER BY p.updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WatchedRow
	for rows.Next() {
		var r WatchedRow
		if err := rows.Scan(&r.Profile, &r.Title, &r.Year, &r.Season, &r.Episode, &r.Date); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Backup writes a consistent copy of the database to path.
func (s *Store) Backup(path string) error {
	_, err := s.DB.Exec(`VACUUM INTO ?`, path)
	return err
}
