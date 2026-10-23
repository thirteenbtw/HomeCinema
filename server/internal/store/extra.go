package store

import "encoding/json"

// ---------- devices ----------

type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Model  string `json:"model"`
	Proto  string `json:"proto"`
	Addr   string `json:"addr"`
	Port   int    `json:"port"`
	Manual bool   `json:"manual"`
}

func (s *Store) Devices() ([]Device, error) {
	rows, err := s.DB.Query(`SELECT id, name, model, proto, addr, port, manual FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Name, &d.Model, &d.Proto, &d.Addr, &d.Port, &d.Manual); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) SaveDevice(d Device) error {
	_, err := s.Exec(`INSERT INTO devices(id, name, model, proto, addr, port, manual) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, model=excluded.model, addr=excluded.addr, port=excluded.port`,
		d.ID, d.Name, d.Model, d.Proto, d.Addr, d.Port, d.Manual)
	return err
}

func (s *Store) DeleteDevice(id string) error {
	_, err := s.Exec(`DELETE FROM devices WHERE id=?`, id)
	return err
}

// ---------- torrents ----------

type TorrentRow struct {
	InfoHash string
	Name     string
	Source   string
	Mode     string
	Paused   bool
	Keep     bool
	Priority int
	FilesOff []int
	Imported bool
	AddedAt  int64
	DoneAt   int64
}

func (s *Store) Torrents() ([]TorrentRow, error) {
	rows, err := s.DB.Query(`SELECT infohash, name, source, mode, paused, keep, priority, files_off, imported, added_at, done_at FROM torrents ORDER BY priority DESC, added_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TorrentRow
	for rows.Next() {
		var t TorrentRow
		var off string
		if err := rows.Scan(&t.InfoHash, &t.Name, &t.Source, &t.Mode, &t.Paused, &t.Keep, &t.Priority, &off, &t.Imported, &t.AddedAt, &t.DoneAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(off), &t.FilesOff)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveTorrent(t TorrentRow) error {
	if t.AddedAt == 0 {
		t.AddedAt = now()
	}
	if t.FilesOff == nil {
		t.FilesOff = []int{}
	}
	_, err := s.Exec(`INSERT INTO torrents(infohash, name, source, mode, paused, keep, priority, files_off, imported, added_at, done_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(infohash) DO UPDATE SET name=excluded.name, mode=excluded.mode, paused=excluded.paused, keep=excluded.keep, priority=excluded.priority,
		files_off=excluded.files_off, imported=excluded.imported, done_at=excluded.done_at`,
		t.InfoHash, t.Name, t.Source, t.Mode, t.Paused, t.Keep, t.Priority, toJSON(t.FilesOff), t.Imported, t.AddedAt, t.DoneAt)
	return err
}

func (s *Store) DeleteTorrent(hash string) error {
	_, err := s.Exec(`DELETE FROM torrents WHERE infohash=?`, hash)
	return err
}
