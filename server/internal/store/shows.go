package store

import "database/sql"

// Shows are found by the name parsed from the files (folder_title), because
// title is replaced by the TMDB name in the metadata language once matched.
// Looking them up by title alone made every episode indexed after the match
// create a second show row, which then could not be renamed (UNIQUE title/year).

// migrate brings databases created by older versions up to date.
func (s *Store) migrate() error {
	cols := map[string]bool{}
	rows, err := s.DB.Query(`PRAGMA table_info(shows)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		cols[name] = true
	}
	rows.Close()
	if !cols["folder_title"] {
		if _, err := s.Exec(`ALTER TABLE shows ADD COLUMN folder_title TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !cols["folder_year"] {
		if _, err := s.Exec(`ALTER TABLE shows ADD COLUMN folder_year INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// Episodes keep the parsed show name in their own title, so the folder
	// name of old rows can be recovered from them.
	if _, err := s.Exec(`UPDATE shows SET
		folder_title=IFNULL((SELECT m.title FROM media_items m WHERE m.show_id=shows.id AND m.kind='episode' ORDER BY m.id LIMIT 1), title),
		folder_year=CASE WHEN matched=0 THEN year ELSE 0 END
		WHERE folder_title=''`); err != nil {
		return err
	}
	return s.MergeDuplicateShows()
}

// MergeDuplicateShows folds rows that describe the same show (same TMDB id,
// or the same folder name) into one, keeping the matched row.
func (s *Store) MergeDuplicateShows() error {
	type row struct {
		id, tmdb      int64
		folder        string
		year, matched int
	}
	rows, err := s.DB.Query(`SELECT id, tmdb_id, lower(folder_title), folder_year, matched FROM shows
		ORDER BY CASE WHEN matched IN (1,3) THEN 0 ELSE 1 END, id`)
	if err != nil {
		return err
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.tmdb, &r.folder, &r.year, &r.matched); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	var kept []row
	for _, r := range all {
		into := int64(0)
		for _, k := range kept {
			if (r.tmdb > 0 && r.tmdb == k.tmdb) ||
				(r.folder != "" && r.folder == k.folder && (r.year == k.year || r.year == 0 || k.year == 0) && (r.tmdb == 0 || k.tmdb == 0)) {
				into = k.id
				break
			}
		}
		if into == 0 {
			kept = append(kept, r)
			continue
		}
		if err := s.mergeShow(r.id, into); err != nil {
			return err
		}
	}
	return nil
}

// mergeShow moves episodes and list entries of show from into show into and
// deletes from. Moved episodes are matched again against the kept show.
func (s *Store) mergeShow(from, into int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := mergeShowTx(tx, from, into); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeShowTx(tx *sql.Tx, from, into int64) error {
	for _, q := range []string{
		`UPDATE media_items SET show_id=?, matched=0 WHERE show_id=?`,
		`UPDATE OR IGNORE lists SET show_id=? WHERE show_id=?`,
	} {
		if _, err := tx.Exec(q, into, from); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM lists WHERE show_id=?`, from); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM shows WHERE id=?`, from)
	return err
}

// UpsertShow finds or creates a show by parsed title/year and returns its id.
func (s *Store) UpsertShow(title string, year int) (int64, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM shows
		WHERE (lower(folder_title)=lower(?1) AND (folder_year=?2 OR ?2=0 OR folder_year=0))
		   OR (lower(title)=lower(?1) AND (year=?2 OR ?2=0 OR year=0))
		ORDER BY lower(folder_title)=lower(?1) DESC, id LIMIT 1`, title, year).Scan(&id)
	if err == nil {
		return id, nil
	}
	res, err := s.Exec(`INSERT INTO shows(title, year, folder_title, folder_year) VALUES(?,?,?,?)`, title, year, title, year)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ApplyShowMeta stores TMDB metadata on a show. If another row already holds
// the same TMDB show, or the TMDB title/year, the two rows are merged instead
// of failing on the unique key.
func (s *Store) ApplyShowMeta(id int64, m Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title string
	var year int
	if err := tx.QueryRow(`SELECT title, year FROM shows WHERE id=?`, id).Scan(&title, &year); err != nil {
		return err
	}
	if m.Title != "" {
		title = m.Title
	}
	if m.Year > 0 {
		year = m.Year
	}
	var other, otherTmdb int64
	err = tx.QueryRow(`SELECT id, tmdb_id FROM shows WHERE id<>?1 AND ((tmdb_id=?2 AND ?2>0) OR (title=?3 AND year=?4))
		ORDER BY tmdb_id=?2 DESC LIMIT 1`, id, m.TmdbID, title, year).Scan(&other, &otherTmdb)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return err
	case otherTmdb == m.TmdbID || otherTmdb == 0:
		// The same show under another row: keep this one, fold the other in.
		if err := mergeShowTx(tx, other, id); err != nil {
			return err
		}
	default:
		// A different show already has this name: keep the folder name.
		if err := tx.QueryRow(`SELECT folder_title, CASE WHEN folder_year>0 THEN folder_year ELSE year END FROM shows WHERE id=?`, id).Scan(&title, &year); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE shows SET tmdb_id=?, title=?, year=?,
		overview=?, poster=?, backdrop=?, genres=?, rating=?, cast_json=?, matched=? WHERE id=?`,
		m.TmdbID, title, year, m.Overview, m.Poster, m.Backdrop, toJSON(nzs(m.Genres)), m.Rating, toJSON(nzc(m.Cast)), m.Matched, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
