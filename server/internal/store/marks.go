package store

import "fmt"

// ---------- marks: likes, watch later and watched for catalog titles ----------

type Mark struct {
	List    string `json:"list"`
	Kind    string `json:"kind"` // movie | tv
	ID      int64  `json:"id"`   // TMDB id
	Title   string `json:"title"`
	Orig    string `json:"origTitle"`
	Year    int    `json:"year"`
	Poster  string `json:"poster"`
	Rating  int    `json:"rating"` // 1–10, 0 if not rated
	Source  string `json:"source"`
	AddedAt int64  `json:"addedAt"`
}

func ValidMarkList(l string) bool { return l == "liked" || l == "later" || l == "watched" }

// SetMark adds or removes a title from a list. Adding again keeps the
// original date and only refreshes the snapshot (and the rating when given).
func (s *Store) SetMark(pid int64, m Mark, on bool) error {
	if !ValidMarkList(m.List) {
		return fmt.Errorf("неизвестный список %q", m.List)
	}
	if m.Kind != "tv" {
		m.Kind = "movie"
	}
	if !on {
		_, err := s.Exec(`DELETE FROM marks WHERE profile_id=? AND list=? AND kind=? AND tmdb_id=?`, pid, m.List, m.Kind, m.ID)
		return err
	}
	if m.AddedAt == 0 {
		m.AddedAt = now()
	}
	_, err := s.Exec(`INSERT INTO marks(profile_id, list, kind, tmdb_id, title, orig_title, year, poster, rating, source, added_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(profile_id, list, kind, tmdb_id) DO UPDATE SET
		  title=CASE WHEN excluded.title<>'' THEN excluded.title ELSE title END,
		  orig_title=CASE WHEN excluded.orig_title<>'' THEN excluded.orig_title ELSE orig_title END,
		  year=CASE WHEN excluded.year>0 THEN excluded.year ELSE year END,
		  poster=CASE WHEN excluded.poster<>'' THEN excluded.poster ELSE poster END,
		  rating=CASE WHEN excluded.rating>0 THEN excluded.rating ELSE rating END`,
		pid, m.List, m.Kind, m.ID, m.Title, m.Orig, m.Year, m.Poster, m.Rating, m.Source, m.AddedAt)
	return err
}

// Marks lists one list of a profile, newest first; an empty list means all.
func (s *Store) Marks(pid int64, list string) ([]Mark, error) {
	q := `SELECT list, kind, tmdb_id, title, orig_title, year, poster, rating, source, added_at FROM marks WHERE profile_id=?`
	args := []any{pid}
	if list != "" {
		q += ` AND list=?`
		args = append(args, list)
	}
	rows, err := s.DB.Query(q+` ORDER BY added_at DESC, rowid DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mark{}
	for rows.Next() {
		var m Mark
		if err := rows.Scan(&m.List, &m.Kind, &m.ID, &m.Title, &m.Orig, &m.Year, &m.Poster, &m.Rating, &m.Source, &m.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LBMatch returns the remembered TMDB film for a Letterboxd name and year;
// a match with ID 0 means TMDB had nothing for it.
func (s *Store) LBMatch(name string, year int) (Mark, bool) {
	m := Mark{Kind: "movie"}
	err := s.DB.QueryRow(`SELECT tmdb_id, title, orig_title, tmdb_year, poster FROM lb_match WHERE name=? AND year=?`, name, year).
		Scan(&m.ID, &m.Title, &m.Orig, &m.Year, &m.Poster)
	return m, err == nil
}

func (s *Store) SetLBMatch(name string, year int, m Mark) error {
	_, err := s.Exec(`INSERT INTO lb_match(name, year, tmdb_id, title, orig_title, tmdb_year, poster) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(name, year) DO UPDATE SET tmdb_id=excluded.tmdb_id, title=excluded.title, orig_title=excluded.orig_title,
		tmdb_year=excluded.tmdb_year, poster=excluded.poster`, name, year, m.ID, m.Title, m.Orig, m.Year, m.Poster)
	return err
}

// ClearMarks removes what came from one source (e.g. a Letterboxd import).
func (s *Store) ClearMarks(pid int64, source string) (int64, error) {
	res, err := s.Exec(`DELETE FROM marks WHERE profile_id=? AND source=?`, pid, source)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
