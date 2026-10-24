package api

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/netutil"
	"github.com/thirteenbtw/homecinema/server/internal/torrent"
)

var generalKeys = []string{"metaLang", "uiLang", "hwaccel", "quality", "maxStreams", "tonemap", "externalPlayer", "useExternal", "setupDone", "subLang", "autoNext", "tmdbApi", "tmdbImg", "tmdbVia"}

var generalDefaults = map[string]string{"metaLang": "ru-RU", "uiLang": "ru", "hwaccel": "auto", "quality": "original", "maxStreams": "2", "tonemap": "1", "useExternal": "", "setupDone": "", "subLang": "rus", "autoNext": "1", "tmdbVia": "cub"}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	out := map[string]any{}
	for _, k := range generalKeys {
		out[k] = s.Store.Setting(k, generalDefaults[k])
	}
	key := s.Store.Setting("tmdbKey", "")
	out["tmdbKeySet"] = key != ""
	if key != "" {
		out["tmdbKeyTail"] = tail(key, 4)
	}
	out["passwordSet"] = s.Store.Setting("password", "") != ""
	out["hw"] = s.Trans.HW()
	out["encoder"] = s.Trans.Encoder()
	return writeJSON(w, out)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	var b map[string]any
	if err := readJSON(r, &b); err != nil {
		return err
	}
	oldAPI, oldImg, oldVia := s.Store.Setting("tmdbApi", ""), s.Store.Setting("tmdbImg", ""), s.Store.Setting("tmdbVia", generalDefaults["tmdbVia"])
	for _, k := range generalKeys {
		if v, ok := b[k]; ok {
			if err := s.Store.SetSetting(k, fmt.Sprint(v)); err != nil {
				return err
			}
		}
	}
	if v, ok := b["tmdbKey"].(string); ok {
		v = strings.TrimSpace(v)
		if err := s.Store.SetSetting("tmdbKey", v); err != nil {
			return err
		}
		if v != "" {
			if err := s.TMDB.Check(r.Context()); err != nil {
				if strings.Contains(err.Error(), "нет связи") {
					return badRequest("ключ сохранён, но проверить его не удалось: %v", err)
				}
				return badRequest("ключ TMDB не подошёл: %v", err)
			}
			s.Store.ResetUnmatched()
			s.Scanner.KickMetadata()
		}
	}
	_, api := b["tmdbApi"]
	_, img := b["tmdbImg"]
	_, via := b["tmdbVia"]
	if (api || img || via) && s.Store.Setting("tmdbKey", "") != "" {
		if err := s.TMDB.Check(r.Context()); err != nil {
			_ = s.Store.SetSetting("tmdbApi", oldAPI)
			_ = s.Store.SetSetting("tmdbImg", oldImg)
			_ = s.Store.SetSetting("tmdbVia", oldVia)
			return badRequest("TMDB так не отвечает, настройка не изменена: %v", err)
		}
		s.Store.ResetUnmatched()
		s.Scanner.KickMetadata()
	}
	if v, ok := b["metaLang"]; ok && v != "" {
		s.Scanner.KickMetadata()
	}
	if v, ok := b["password"].(string); ok {
		h := ""
		if v != "" {
			h = hashSecret(hex.EncodeToString(s.secret), v)
		}
		if err := s.Store.SetSetting("password", h); err != nil {
			return err
		}
		if v != "" { // keep the person who set it signed in
			http.SetCookie(w, &http.Cookie{Name: "hc_auth", Value: s.sign("auth", "1", 90*24*time.Hour), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 90 * 24 * 3600})
		}
	}
	return s.getSettings(w, r)
}

// ---------- libraries (FR-1) ----------

func (s *Server) libraries(w http.ResponseWriter, r *http.Request) error {
	libs, err := s.Store.Libraries()
	if err != nil {
		return err
	}
	type libOut struct {
		ID     int64  `json:"id"`
		Path   string `json:"path"`
		Kind   string `json:"kind"`
		Online bool   `json:"online"`
	}
	out := []libOut{}
	for _, l := range libs {
		_, err := os.Stat(l.Path)
		out = append(out, libOut{l.ID, l.Path, l.Kind, err == nil})
	}
	return writeJSON(w, map[string]any{"libraries": out, "scan": s.Scanner.Status()})
}

func (s *Server) addLibrary(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	p := filepath.Clean(strings.TrimSpace(b.Path))
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		return badRequest("папка не найдена: %s", p)
	}
	switch b.Kind {
	case "", "auto", "movies", "shows", "torrents":
	default:
		return badRequest("неизвестный тип папки")
	}
	lib, err := s.Store.AddLibrary(p, b.Kind)
	if err != nil {
		return err
	}
	s.Scanner.RequestScan(lib.ID)
	return writeJSON(w, lib)
}

func (s *Server) removeLibrary(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	return s.okOr(w, s.Store.RemoveLibrary(id))
}

func (s *Server) rescan(w http.ResponseWriter, r *http.Request) error {
	s.Scanner.RequestScan(0)
	return writeJSON(w, map[string]bool{"ok": true})
}

// browseFS lists folders for the "add folder" dialog. It runs on the server
// PC, so it is limited to the local window and to authorised LAN clients.
func (s *Server) browseFS(w http.ResponseWriter, r *http.Request) error {
	p := r.URL.Query().Get("path")
	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	out := struct {
		Path   string  `json:"path"`
		Parent string  `json:"parent"`
		Dirs   []entry `json:"dirs"`
	}{Dirs: []entry{}}
	if p == "" {
		if runtime.GOOS == "windows" {
			for c := 'A'; c <= 'Z'; c++ {
				d := string(c) + `:\`
				if _, err := os.Stat(d); err == nil {
					out.Dirs = append(out.Dirs, entry{d, d})
				}
			}
		} else {
			home, _ := os.UserHomeDir()
			out.Dirs = append(out.Dirs, entry{"Домашняя папка", home}, entry{"/", "/"})
			if _, err := os.Stat("/media"); err == nil {
				out.Dirs = append(out.Dirs, entry{"/media", "/media"})
			}
		}
		return writeJSON(w, out)
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		return badRequest("%v", err)
	}
	out.Path = p
	if parent := filepath.Dir(p); parent != p {
		out.Parent = parent
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(e.Name(), "$") {
			out.Dirs = append(out.Dirs, entry{e.Name(), filepath.Join(p, e.Name())})
		}
	}
	sort.Slice(out.Dirs, func(i, j int) bool { return strings.ToLower(out.Dirs[i].Name) < strings.ToLower(out.Dirs[j].Name) })
	return writeJSON(w, out)
}

// ---------- diagnostics (FR-23) ----------

type check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn"`
	Detail string `json:"detail"`
}

var tmdbCheck struct {
	sync.Mutex
	at  time.Time
	err error
	key string
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) error {
	var out []check
	hw := s.Trans.HW()
	if hw.FFmpegFound {
		d := "ffmpeg " + hw.Version
		if len(hw.Encoders) > 0 {
			d += " · " + strings.ToUpper(strings.Join(hw.Encoders, ", ")) + " доступно"
		} else {
			d += " · только CPU"
		}
		out = append(out, check{"ffmpeg", "ffmpeg найден", true, false, d})
	} else {
		out = append(out, check{"ffmpeg", "ffmpeg не найден", false, false, "Положите ffmpeg.exe рядом с программой или укажите путь"})
	}
	lan := netutil.LANAddrs()
	if len(lan) > 0 {
		out = append(out, check{"server", "Сервер в LAN", true, false, fmt.Sprintf("%s:%d · только локальная сеть", lan[0], s.Port)})
	} else {
		out = append(out, check{"server", "Нет сети", false, true, "ТВ не смогут подключиться"})
	}
	devs := s.Cast.Devices()
	online := 0
	for _, d := range devs {
		if d.Online {
			online++
		}
	}
	out = append(out, check{"tv", "ТВ в сети", online > 0, online == 0, fmt.Sprintf("Google Cast: доступно %d из %d", online, len(devs))})

	key := s.Store.Setting("tmdbKey", "")
	if key == "" {
		out = append(out, check{"tmdb", "TMDB", false, true, "Ключ не задан — постеры и описания не подтянутся"})
	} else {
		tmdbCheck.Lock()
		if time.Since(tmdbCheck.at) > time.Minute || tmdbCheck.key != key {
			tmdbCheck.err = s.TMDB.Check(r.Context())
			tmdbCheck.at, tmdbCheck.key = time.Now(), key
		}
		err := tmdbCheck.err
		tmdbCheck.Unlock()
		size := dirSize(filepath.Join(s.Cfg.DataDir, "images"))
		if err != nil {
			out = append(out, check{"tmdb", "TMDB", false, true, err.Error() + fmt.Sprintf(" · кэш %.1f ГБ, работаем офлайн", float64(size)/(1<<30))})
		} else {
			out = append(out, check{"tmdb", "TMDB", true, false, fmt.Sprintf("Ключ работает · кэш %.1f ГБ", float64(size)/(1<<30))})
		}
	}

	tov := s.Torrents.List()
	ts := s.Torrents.Settings()
	switch {
	case !tov.Running:
		out = append(out, check{"port", "Торрент-движок", false, false, tov.Error})
	default:
		ok := portListening(ts.Port)
		detail := fmt.Sprintf("Порт %d слушается", ts.Port)
		if ts.UPnP {
			detail += " · UPnP вкл. — если входящих нет, откройте порт на роутере"
		} else {
			detail += " · UPnP выкл. — откройте порт вручную"
		}
		out = append(out, check{"port", "Входящий порт торрентов", ok, !ok, detail})
	}
	if ts.BindIface != "" {
		up := false
		ip := ""
		for _, i := range torrent.Interfaces() {
			if i.Name == ts.BindIface && i.IP != "" {
				up, ip = true, i.IP
			}
		}
		d := ts.BindIface + " · " + map[bool]string{true: "активен " + ip, false: "недоступен"}[up]
		if ts.KillSwitch {
			d += " · kill switch вкл."
		}
		out = append(out, check{"vpn", "VPN и kill switch", up, !up, d})
	} else {
		out = append(out, check{"vpn", "VPN", true, true, "Торренты работают через любой интерфейс"})
	}
	if tov.FreeGB > 0 {
		low := tov.DiskLow
		out = append(out, check{"disk", "Место на диске", !low, low, fmt.Sprintf("Свободно %.0f ГБ в папке загрузок", tov.FreeGB)})
	}
	return writeJSON(w, map[string]any{"checks": out, "sessions": s.Trans.Sessions(), "version": s.Version, "dataDir": s.Cfg.DataDir})
}

func portListening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, err := d.Info(); err == nil {
				n += i.Size()
			}
		}
		return nil
	})
	return n
}

// ---------- backup (FR-22) ----------

func (s *Server) backup(w http.ResponseWriter, r *http.Request) error {
	tmp := filepath.Join(s.Cfg.CacheDir, fmt.Sprintf("backup-%d.db", time.Now().UnixNano()))
	defer os.Remove(tmp)
	if err := s.Store.Backup(tmp); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="rivo-%s.db"`, time.Now().Format("2006-01-02")))
	http.ServeFile(w, r, tmp)
	return nil
}

// restore stages an uploaded database; it replaces the current one on the
// next start (the running server keeps its connection open).
func (s *Server) restore(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		return badRequest("%v", err)
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		return badRequest("нужен файл резервной копии")
	}
	defer f.Close()
	head := make([]byte, 16)
	if _, err := io.ReadFull(f, head); err != nil || !bytes.HasPrefix(head, []byte("SQLite format 3")) {
		return badRequest("это не резервная копия Rivo")
	}
	dst := filepath.Join(s.Cfg.DataDir, "restore.db")
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	out.Write(head)
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"ok": true, "restart": true})
}

func (s *Server) exportWatched(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.Store.WatchedExport()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="watched.csv"`)
	w.Write([]byte("\xEF\xBB\xBF")) // BOM so Excel reads Cyrillic correctly
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Профиль", "Название", "Год", "Сезон", "Серия", "Дата"})
	for _, x := range rows {
		_ = cw.Write([]string{x.Profile, x.Title, strconv.Itoa(x.Year), strconv.Itoa(x.Season), strconv.Itoa(x.Episode), time.Unix(x.Date, 0).Format("2006-01-02 15:04")})
	}
	cw.Flush()
	return cw.Error()
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) error {
	p := filepath.Join(s.Cfg.DataDir, "logs", "homecinema.log")
	f, err := os.Open(p)
	if err != nil {
		return writeJSON(w, map[string]any{"lines": []string{}, "path": p})
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > 500 {
			lines = lines[len(lines)-400:]
		}
	}
	return writeJSON(w, map[string]any{"lines": lines, "path": p})
}

func toJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
