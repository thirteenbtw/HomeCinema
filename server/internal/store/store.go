// Package store wraps the SQLite database: library, metadata, profiles, progress.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	DB *sql.DB
	mu sync.Mutex // serialises writes; SQLite allows one writer anyway
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.ensureDefaultProfile(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Exec(q string, args ...any) (sql.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Exec(q, args...)
}

func now() int64 { return time.Now().Unix() }

// ---------- settings ----------

func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) AllSettings() map[string]string {
	out := map[string]string{}
	rows, err := s.DB.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

// ---------- libraries ----------

type Library struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (s *Store) Libraries() ([]Library, error) {
	rows, err := s.DB.Query(`SELECT id, path, kind FROM libraries ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Library
	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Path, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) AddLibrary(path, kind string) (Library, error) {
	if kind == "" {
		kind = "auto"
	}
	_, err := s.Exec(`INSERT INTO libraries(path, kind) VALUES(?,?) ON CONFLICT(path) DO UPDATE SET kind=excluded.kind`, path, kind)
	if err != nil {
		return Library{}, err
	}
	var l Library
	err = s.DB.QueryRow(`SELECT id, path, kind FROM libraries WHERE path=?`, path).Scan(&l.ID, &l.Path, &l.Kind)
	return l, err
}

func (s *Store) RemoveLibrary(id int64) error {
	_, err := s.Exec(`DELETE FROM libraries WHERE id=?`, id)
	if err == nil {
		s.PruneShows()
	}
	return err
}

// ---------- profiles ----------

type Profile struct {
	ID     int64           `json:"id"`
	Name   string          `json:"name"`
	HasPIN bool            `json:"hasPin"`
	Color  string          `json:"color"`
	Prefs  json.RawMessage `json:"prefs"`
	IsKid  bool            `json:"isKid"`
}

const MaxProfiles = 5

func (s *Store) ensureDefaultProfile() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err := s.Exec(`INSERT INTO profiles(name, color) VALUES('Владелец', '#5B6EE1')`)
		return err
	}
	return nil
}

func (s *Store) Profiles() ([]Profile, error) {
	rows, err := s.DB.Query(`SELECT id, name, pin<>'', color, prefs, is_kid FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var prefs string
		if err := rows.Scan(&p.ID, &p.Name, &p.HasPIN, &p.Color, &prefs, &p.IsKid); err != nil {
			return nil, err
		}
		p.Prefs = json.RawMessage(prefs)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ProfilePIN(id int64) (string, error) {
	var pin string
	err := s.DB.QueryRow(`SELECT pin FROM profiles WHERE id=?`, id).Scan(&pin)
	return pin, err
}

func (s *Store) CreateProfile(name, color, pinHash string, kid bool) (int64, error) {
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&n)
	if n >= MaxProfiles {
		return 0, fmt.Errorf("не больше %d профилей", MaxProfiles)
	}
	res, err := s.Exec(`INSERT INTO profiles(name, color, pin, is_kid) VALUES(?,?,?,?)`, name, color, pinHash, kid)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateProfile(id int64, name, color string, prefs json.RawMessage, pinHash *string, kid *bool) error {
	if name != "" {
		if _, err := s.Exec(`UPDATE profiles SET name=? WHERE id=?`, name, id); err != nil {
			return err
		}
	}
	if color != "" {
		if _, err := s.Exec(`UPDATE profiles SET color=? WHERE id=?`, color, id); err != nil {
			return err
		}
	}
	if len(prefs) > 0 {
		if _, err := s.Exec(`UPDATE profiles SET prefs=? WHERE id=?`, string(prefs), id); err != nil {
			return err
		}
	}
	if pinHash != nil {
		if _, err := s.Exec(`UPDATE profiles SET pin=? WHERE id=?`, *pinHash, id); err != nil {
			return err
		}
	}
	if kid != nil {
		if _, err := s.Exec(`UPDATE profiles SET is_kid=? WHERE id=?`, *kid, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteProfile(id int64) error {
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&n)
	if n <= 1 {
		return fmt.Errorf("нельзя удалить последний профиль")
	}
	_, err := s.Exec(`DELETE FROM profiles WHERE id=?`, id)
	return err
}

// ---------- helpers ----------

func jsonStrings(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
