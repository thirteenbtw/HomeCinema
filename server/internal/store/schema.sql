PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS libraries (
  id      INTEGER PRIMARY KEY,
  path    TEXT NOT NULL UNIQUE,
  kind    TEXT NOT NULL DEFAULT 'auto' -- auto | movies | shows | torrents
);

CREATE TABLE IF NOT EXISTS shows (
  id        INTEGER PRIMARY KEY,
  title     TEXT NOT NULL,
  year      INTEGER NOT NULL DEFAULT 0,
  tmdb_id   INTEGER NOT NULL DEFAULT 0,
  overview  TEXT NOT NULL DEFAULT '',
  poster    TEXT NOT NULL DEFAULT '',
  backdrop  TEXT NOT NULL DEFAULT '',
  genres    TEXT NOT NULL DEFAULT '[]',
  rating    REAL NOT NULL DEFAULT 0,
  cast_json TEXT NOT NULL DEFAULT '[]',
  matched   INTEGER NOT NULL DEFAULT 0,
  folder_title TEXT NOT NULL DEFAULT '', -- name parsed from the files; title becomes the TMDB one
  folder_year  INTEGER NOT NULL DEFAULT 0,
  UNIQUE(title, year)
);

-- media_items: one row per playable file (a movie or an episode).
CREATE TABLE IF NOT EXISTS media_items (
  id          INTEGER PRIMARY KEY,
  library_id  INTEGER REFERENCES libraries(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,            -- movie | episode
  title       TEXT NOT NULL,
  orig_title  TEXT NOT NULL DEFAULT '',
  year        INTEGER NOT NULL DEFAULT 0,
  show_id     INTEGER REFERENCES shows(id) ON DELETE SET NULL,
  season      INTEGER NOT NULL DEFAULT 0,
  episode     INTEGER NOT NULL DEFAULT 0,
  ep_title    TEXT NOT NULL DEFAULT '',
  tmdb_id     INTEGER NOT NULL DEFAULT 0,
  overview    TEXT NOT NULL DEFAULT '',
  poster      TEXT NOT NULL DEFAULT '',
  backdrop    TEXT NOT NULL DEFAULT '',
  genres      TEXT NOT NULL DEFAULT '[]',
  rating      REAL NOT NULL DEFAULT 0,
  runtime     INTEGER NOT NULL DEFAULT 0,
  cert        TEXT NOT NULL DEFAULT '',
  cast_json   TEXT NOT NULL DEFAULT '[]',
  matched     INTEGER NOT NULL DEFAULT 0,   -- 0 pending, 1 matched, 2 not found, 3 manual
  added_at    INTEGER NOT NULL,
  source      TEXT NOT NULL DEFAULT 'folder' -- folder | torrent
);
CREATE INDEX IF NOT EXISTS idx_items_show ON media_items(show_id, season, episode);
CREATE INDEX IF NOT EXISTS idx_items_added ON media_items(added_at);

CREATE TABLE IF NOT EXISTS files (
  id          INTEGER PRIMARY KEY,
  item_id     INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  path        TEXT NOT NULL UNIQUE,
  size        INTEGER NOT NULL DEFAULT 0,
  mtime       INTEGER NOT NULL DEFAULT 0,
  container   TEXT NOT NULL DEFAULT '',
  duration    REAL NOT NULL DEFAULT 0,
  video_codec TEXT NOT NULL DEFAULT '',
  width       INTEGER NOT NULL DEFAULT 0,
  height      INTEGER NOT NULL DEFAULT 0,
  bit_depth   INTEGER NOT NULL DEFAULT 8,
  hdr         TEXT NOT NULL DEFAULT '',
  audio_json  TEXT NOT NULL DEFAULT '[]',
  subs_json   TEXT NOT NULL DEFAULT '[]',
  probed      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS profiles (
  id      INTEGER PRIMARY KEY,
  name    TEXT NOT NULL,
  pin     TEXT NOT NULL DEFAULT '',
  color   TEXT NOT NULL DEFAULT '#C8232D',
  prefs   TEXT NOT NULL DEFAULT '{}',      -- theme, accent, background
  is_kid  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS watch_progress (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  position   REAL NOT NULL DEFAULT 0,
  duration   REAL NOT NULL DEFAULT 0,
  watched    INTEGER NOT NULL DEFAULT 0,
  device     TEXT NOT NULL DEFAULT 'pc',
  plays      INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (profile_id, item_id)
);

CREATE TABLE IF NOT EXISTS lists (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  list       TEXT NOT NULL,   -- favorites | later
  item_id    INTEGER NOT NULL DEFAULT 0,
  show_id    INTEGER NOT NULL DEFAULT 0,
  added_at   INTEGER NOT NULL,
  PRIMARY KEY (profile_id, list, item_id, show_id)
);

CREATE TABLE IF NOT EXISTS devices (
  id     TEXT PRIMARY KEY,
  name   TEXT NOT NULL,
  model  TEXT NOT NULL DEFAULT '',
  proto  TEXT NOT NULL,       -- cast
  addr   TEXT NOT NULL,
  port   INTEGER NOT NULL,
  manual INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS torrents (
  infohash   TEXT PRIMARY KEY,
  name       TEXT NOT NULL DEFAULT '',
  source     TEXT NOT NULL,             -- magnet URI or base64 .torrent
  mode       TEXT NOT NULL DEFAULT 'download', -- download | stream
  paused     INTEGER NOT NULL DEFAULT 0,
  keep       INTEGER NOT NULL DEFAULT 1, -- save to library after streaming
  priority   INTEGER NOT NULL DEFAULT 0,
  files_off  TEXT NOT NULL DEFAULT '[]', -- indexes of deselected files
  imported   INTEGER NOT NULL DEFAULT 0,
  added_at   INTEGER NOT NULL,
  done_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- marks: catalog titles a profile liked, wants to watch later or has seen,
-- by TMDB id (the title need not be in the library). The title snapshot keeps
-- the lists readable when TMDB cannot be reached.
CREATE TABLE IF NOT EXISTS marks (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  list       TEXT NOT NULL,              -- liked | later | watched
  kind       TEXT NOT NULL,              -- movie | tv
  tmdb_id    INTEGER NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  orig_title TEXT NOT NULL DEFAULT '',
  year       INTEGER NOT NULL DEFAULT 0,
  poster     TEXT NOT NULL DEFAULT '',
  rating     INTEGER NOT NULL DEFAULT 0, -- the profile's own rating, 1–10 (Letterboxd half stars × 2)
  source     TEXT NOT NULL DEFAULT '',   -- '' | letterboxd
  added_at   INTEGER NOT NULL,
  PRIMARY KEY (profile_id, list, kind, tmdb_id)
);
CREATE INDEX IF NOT EXISTS idx_marks_added ON marks(profile_id, list, added_at);

-- lb_match remembers which TMDB film a Letterboxd name and year is (0: none),
-- so a second import of the same export does not search again.
CREATE TABLE IF NOT EXISTS lb_match (
  name       TEXT NOT NULL,
  year       INTEGER NOT NULL,
  tmdb_id    INTEGER NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  orig_title TEXT NOT NULL DEFAULT '',
  tmdb_year  INTEGER NOT NULL DEFAULT 0,
  poster     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (name, year)
);
