// Package torrent is the built-in torrent engine (FR-T1…FR-T10) on top of
// anacrolix/torrent: download queue, streaming with sequential priority and
// HTTP Range, VPN binding with kill switch, auto-import into the library.
package torrent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/net/proxy"
	"golang.org/x/time/rate"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/parse"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

type Settings struct {
	DownloadDir  string  `json:"downloadDir"`
	MoviesDir    string  `json:"moviesDir"`
	ShowsDir     string  `json:"showsDir"`
	Port         int     `json:"port"`
	DownKBs      int     `json:"downKBs"` // 0 = unlimited
	UpKBs        int     `json:"upKBs"`
	SeedRatio    float64 `json:"seedRatio"`   // 0 = do not seed
	SeedMinutes  int     `json:"seedMinutes"` // 0 = no time limit
	BindIface    string  `json:"bindIface"`   // e.g. wg0; "" = any
	KillSwitch   bool    `json:"killSwitch"`
	Encryption   string  `json:"encryption"` // prefer | require | off
	DHT          bool    `json:"dht"`
	PEX          bool    `json:"pex"`
	UPnP         bool    `json:"upnp"`
	Proxy        string  `json:"proxy"` // socks5://user:pass@host:port
	BufferMB     int     `json:"bufferMB"`
	BufferPct    float64 `json:"bufferPct"`
	MinFreeGB    int     `json:"minFreeGB"`
	MaxActive    int     `json:"maxActive"`
	ScheduleFrom int     `json:"scheduleFrom"` // hour 0-23, -1 = off
	ScheduleTo   int     `json:"scheduleTo"`
	AutoImport   bool    `json:"autoImport"`
	AcceptedRisk bool    `json:"acceptedRisk"` // legal notice acknowledged
}

func DefaultSettings(dataDir string) Settings {
	return Settings{
		DownloadDir: filepath.Join(dataDir, "Downloads"), Port: 42069, SeedRatio: 1.5, SeedMinutes: 0,
		Encryption: "prefer", DHT: true, PEX: true, UPnP: true, BufferMB: 80, BufferPct: 3, MinFreeGB: 5,
		MaxActive: 3, ScheduleFrom: -1, ScheduleTo: -1, AutoImport: true,
	}
}

// Executables are never downloaded or opened (FR-T9).
var blockedExts = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true, ".msi": true, ".ps1": true, ".vbs": true,
	".js": true, ".jar": true, ".lnk": true, ".dll": true, ".pif": true, ".hta": true, ".reg": true, ".apk": true, ".sh": true}

func Blocked(name string) bool { return blockedExts[strings.ToLower(filepath.Ext(name))] }

type Importer interface {
	// ImportFile indexes path, registering root as a library folder of kind if needed.
	ImportFile(ctx context.Context, path, root, kind string) error
}

type managed struct {
	t         *torrent.Torrent
	row       store.TorrentRow
	prevRead  int64
	prevWrite int64
	down, up  float64
	streams   map[*streamReader]struct{}
	streamIdx int
	streamPos int64
	seedFrom  time.Time
	imported  []string
	err       string
	queued    bool
	stopped   bool // seeding limit reached
}

type Engine struct {
	Store    *store.Store
	Bus      *events.Bus
	Importer Importer
	DataDir  string
	CacheDir string

	mu       sync.Mutex
	cl       *torrent.Client
	settings Settings
	items    map[string]*managed
	vpnDown  bool
	diskLow  bool
	startErr string
	stor     storage.ClientImplCloser
}

func New(s *store.Store, bus *events.Bus, dataDir, cacheDir string) *Engine {
	return &Engine{Store: s, Bus: bus, DataDir: dataDir, CacheDir: cacheDir, items: map[string]*managed{}}
}

func (e *Engine) Settings() Settings {
	st := DefaultSettings(e.DataDir)
	if raw := e.Store.Setting("torrent", ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &st)
	}
	return st
}

// UpdateSettings saves new settings and restarts the client when needed.
func (e *Engine) UpdateSettings(st Settings) error {
	old := e.Settings()
	b, _ := json.Marshal(st)
	if err := e.Store.SetSetting("torrent", string(b)); err != nil {
		return err
	}
	e.mu.Lock()
	e.settings = st
	e.mu.Unlock()
	// Rate limiters and network options are fixed at client creation.
	if old.DownKBs != st.DownKBs || old.UpKBs != st.UpKBs || old.Port != st.Port || old.BindIface != st.BindIface || old.Proxy != st.Proxy || old.Encryption != st.Encryption ||
		old.DHT != st.DHT || old.PEX != st.PEX || old.UPnP != st.UPnP || old.DownloadDir != st.DownloadDir {
		return e.Restart()
	}
	return nil
}

func (e *Engine) Start(ctx context.Context) {
	if err := e.Restart(); err != nil {
		log.Printf("torrent: %v", err)
	}
	go e.loop(ctx)
}

// Restart (re)creates the client and re-adds stored torrents.
func (e *Engine) Restart() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
	st := e.Settings()
	e.settings = st
	if err := os.MkdirAll(st.DownloadDir, 0o755); err != nil {
		e.startErr = err.Error()
		return err
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg.DataDir = st.DownloadDir
	cfg.ListenPort = st.Port
	cfg.Seed = st.SeedRatio > 0
	cfg.NoDHT = !st.DHT
	cfg.DisablePEX = !st.PEX
	cfg.NoDefaultPortForwarding = !st.UPnP
	cfg.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{
		Preferred:        st.Encryption != "off",
		RequirePreferred: st.Encryption == "require",
	}
	if st.BindIface != "" {
		ip, err := ifaceIPv4(st.BindIface)
		if err != nil {
			e.vpnDown = true
			e.startErr = fmt.Sprintf("интерфейс %s недоступен: %v", st.BindIface, err)
			if st.KillSwitch {
				return errors.New(e.startErr) // kill switch: never fall back to the open interface
			}
		} else {
			cfg.SetListenAddr(net.JoinHostPort(ip.String(), fmt.Sprint(st.Port)))
			cfg.PublicIp4 = ip
			cfg.DisableIPv6 = true
			e.vpnDown = false
		}
	}
	if st.Proxy != "" {
		u, err := url.Parse(st.Proxy)
		if err != nil {
			e.startErr = "прокси: " + err.Error()
			return err
		}
		cfg.HTTPProxy = http.ProxyURL(u)
		cfg.NoDHT = true // UDP does not go through SOCKS5
		cfg.DisableUTP = true
		cfg.DisableTCP = true // all peer connections go through the proxy dialer below
		cfg.AcceptPeerConnections = false
	}
	cfg.UploadRateLimiter = limiter(st.UpKBs)
	cfg.DownloadRateLimiter = limiter(st.DownKBs)
	e.stor = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: st.DownloadDir})
	cfg.DefaultStorage = e.stor
	cl, err := torrent.NewClient(cfg)
	if err != nil && !cfg.DisableIPv6 {
		cfg.DisableIPv6 = true // systems without IPv6
		cl, err = torrent.NewClient(cfg)
	}
	if err != nil {
		e.startErr = err.Error()
		return err
	}
	if st.Proxy != "" {
		u, _ := url.Parse(st.Proxy)
		if d, err := proxy.FromURL(u, proxy.Direct); err == nil {
			if cd, ok := d.(proxy.ContextDialer); ok {
				cl.AddDialer(torrent.NetworkDialer{Network: "tcp", Dialer: cd})
			}
		}
	}
	e.cl = cl
	e.startErr = ""
	rows, _ := e.Store.Torrents()
	for _, r := range rows {
		if err := e.addLocked(r); err != nil {
			log.Printf("torrent re-add %s: %v", r.Name, err)
		}
	}
	return nil
}

func limiter(kbs int) *rate.Limiter {
	if kbs <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(kbs*1024), 256<<10)
}

func (e *Engine) closeLocked() {
	if e.cl != nil {
		e.cl.Close()
		e.cl = nil
	}
	if e.stor != nil {
		e.stor.Close()
		e.stor = nil
	}
	for _, m := range e.items {
		for r := range m.streams {
			r.Close()
		}
	}
	e.items = map[string]*managed{}
}

func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
}

// ---------- adding ----------

// Add accepts a magnet URI, an info hash or the raw bytes of a .torrent file.
func (e *Engine) Add(source string, torrentFile []byte, mode string, keep bool) (string, error) {
	row := store.TorrentRow{Mode: mode, Keep: keep, AddedAt: time.Now().Unix()}
	if mode == "" {
		row.Mode = "download"
	}
	var hash string
	if len(torrentFile) > 0 {
		mi, err := metainfo.Load(strings.NewReader(string(torrentFile)))
		if err != nil {
			return "", fmt.Errorf("не удалось прочитать .torrent: %w", err)
		}
		hash = mi.HashInfoBytes().HexString()
		row.Source = "torrent:" + base64.StdEncoding.EncodeToString(torrentFile)
		if info, err := mi.UnmarshalInfo(); err == nil {
			row.Name = info.BestName()
		}
	} else {
		source = strings.TrimSpace(source)
		if len(source) == 40 && !strings.Contains(source, ":") {
			source = "magnet:?xt=urn:btih:" + source
		}
		spec, err := torrent.TorrentSpecFromMagnetUri(source)
		if err != nil {
			return "", fmt.Errorf("неверная magnet-ссылка: %w", err)
		}
		hash = spec.InfoHash.HexString()
		row.Source = source
		row.Name = spec.DisplayName
	}
	row.InfoHash = hash
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, ok := e.items[hash]; ok {
		// Already present: switching to streaming is allowed.
		if mode == "stream" && m.row.Mode != "stream" {
			m.row.Mode = "stream"
			_ = e.Store.SaveTorrent(m.row)
		}
		return hash, nil
	}
	if e.cl == nil {
		return "", errors.New(e.unavailableReason())
	}
	if err := e.Store.SaveTorrent(row); err != nil {
		return "", err
	}
	return hash, e.addLocked(row)
}

func (e *Engine) unavailableReason() string {
	if e.vpnDown {
		return "VPN недоступен — торренты остановлены (kill switch)"
	}
	if e.startErr != "" {
		return e.startErr
	}
	return "торрент-движок не запущен"
}

func (e *Engine) addLocked(row store.TorrentRow) error {
	var t *torrent.Torrent
	var err error
	if strings.HasPrefix(row.Source, "torrent:") {
		raw, derr := base64.StdEncoding.DecodeString(strings.TrimPrefix(row.Source, "torrent:"))
		if derr != nil {
			return derr
		}
		mi, lerr := metainfo.Load(strings.NewReader(string(raw)))
		if lerr != nil {
			return lerr
		}
		t, err = e.cl.AddTorrent(mi)
	} else {
		t, err = e.cl.AddMagnet(row.Source)
	}
	if err != nil {
		return err
	}
	m := &managed{t: t, row: row, streams: map[*streamReader]struct{}{}, streamIdx: -1}
	e.items[row.InfoHash] = m
	go func() {
		select {
		case <-t.GotInfo():
		case <-t.Closed():
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if m.row.Name == "" || m.row.Name != t.Name() {
			m.row.Name = t.Name()
			_ = e.Store.SaveTorrent(m.row)
		}
		e.applyFilePriorities(m)
	}()
	return nil
}

// applyFilePriorities: wanted files download normally (download mode); in
// stream mode only the reader drives priorities. Executables never download.
func (e *Engine) applyFilePriorities(m *managed) {
	if m.t.Info() == nil {
		return
	}
	off := map[int]bool{}
	for _, i := range m.row.FilesOff {
		off[i] = true
	}
	for i, f := range m.t.Files() {
		switch {
		case Blocked(f.Path()) || off[i]:
			f.SetPriority(torrent.PiecePriorityNone)
		case m.row.Mode == "download" || m.row.Keep && m.row.Mode == "stream" && len(m.streams) == 0:
			f.SetPriority(torrent.PiecePriorityNormal)
		}
	}
}

// ---------- control ----------

func (e *Engine) get(hash string) (*managed, error) {
	m, ok := e.items[strings.ToLower(hash)]
	if !ok {
		return nil, errors.New("раздача не найдена")
	}
	return m, nil
}

func (e *Engine) SetPaused(hash string, paused bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return err
	}
	m.row.Paused = paused
	m.stopped = false
	return e.Store.SaveTorrent(m.row)
}

func (e *Engine) SetPriority(hash string, prio int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return err
	}
	m.row.Priority = prio
	return e.Store.SaveTorrent(m.row)
}

func (e *Engine) SetMode(hash, mode string, keep *bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return err
	}
	if mode != "" {
		m.row.Mode = mode
	}
	if keep != nil {
		m.row.Keep = *keep
	}
	e.applyFilePriorities(m)
	return e.Store.SaveTorrent(m.row)
}

func (e *Engine) SetFiles(hash string, off []int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return err
	}
	m.row.FilesOff = off
	e.applyFilePriorities(m)
	return e.Store.SaveTorrent(m.row)
}

// Remove drops a torrent; deleteData also removes the downloaded files (not
// the copies already imported into the library).
func (e *Engine) Remove(hash string, deleteData bool) error {
	e.mu.Lock()
	m, err := e.get(hash)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	delete(e.items, m.row.InfoHash)
	var paths []string
	if m.t.Info() != nil {
		for _, f := range m.t.Files() {
			paths = append(paths, filepath.Join(e.settings.DownloadDir, filepath.FromSlash(f.Path())))
		}
	}
	name := m.t.Name()
	multi := m.t.Info() != nil && len(m.t.Info().Files) > 0
	m.t.Drop()
	e.mu.Unlock()
	if deleteData {
		for _, p := range paths {
			_ = os.Remove(p)
		}
		if multi && name != "" {
			_ = os.RemoveAll(filepath.Join(e.settings.DownloadDir, name))
		}
	}
	return e.Store.DeleteTorrent(m.row.InfoHash)
}

// ---------- loop: speeds, queue, schedule, seeding, import, kill switch ----------

func (e *Engine) loop(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			e.Close()
			return
		case <-tick.C:
		}
		n++
		e.checkVPN()
		if n%10 == 0 {
			e.checkDisk()
		}
		e.step()
		e.Bus.Publish("torrents", e.List())
	}
}

func (e *Engine) checkVPN() {
	e.mu.Lock()
	st := e.settings
	down := e.vpnDown
	running := e.cl != nil
	e.mu.Unlock()
	if st.BindIface == "" || !st.KillSwitch {
		return
	}
	_, err := ifaceIPv4(st.BindIface)
	switch {
	case err != nil && running:
		log.Printf("torrent: %s down, kill switch", st.BindIface)
		e.mu.Lock()
		e.closeLocked()
		e.vpnDown = true
		e.startErr = "VPN " + st.BindIface + " недоступен — все торренты остановлены"
		e.mu.Unlock()
	case err == nil && (down || !running):
		log.Printf("torrent: %s is back", st.BindIface)
		_ = e.Restart()
	}
}

func (e *Engine) checkDisk() {
	st := e.Settings()
	free, err := FreeSpace(st.DownloadDir)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.diskLow = err == nil && st.MinFreeGB > 0 && free < uint64(st.MinFreeGB)<<30
}

func inSchedule(st Settings, now time.Time) bool {
	if st.ScheduleFrom < 0 || st.ScheduleTo < 0 || st.ScheduleFrom == st.ScheduleTo {
		return true
	}
	h := now.Hour()
	if st.ScheduleFrom < st.ScheduleTo {
		return h >= st.ScheduleFrom && h < st.ScheduleTo
	}
	return h >= st.ScheduleFrom || h < st.ScheduleTo
}

func (e *Engine) step() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cl == nil {
		return
	}
	st := e.settings
	scheduled := inSchedule(st, time.Now())
	// Order: priority, then age. Streams always run.
	ms := make([]*managed, 0, len(e.items))
	for _, m := range e.items {
		ms = append(ms, m)
	}
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].row.Priority != ms[j].row.Priority {
			return ms[i].row.Priority > ms[j].row.Priority
		}
		return ms[i].row.AddedAt < ms[j].row.AddedAt
	})
	active := 0
	for _, m := range ms {
		s := m.t.Stats()
		read, written := s.BytesReadData.Int64(), s.BytesWrittenData.Int64()
		m.down = smooth(m.down, float64(read-m.prevRead))
		m.up = smooth(m.up, float64(written-m.prevWrite))
		m.prevRead, m.prevWrite = read, written

		complete := m.t.Info() != nil && m.t.BytesMissing() == 0 && wantedComplete(m)
		streaming := len(m.streams) > 0 || m.row.Mode == "stream"
		allowDown := !m.row.Paused && !e.diskLow
		if !complete && !streaming && allowDown {
			if !scheduled || active >= max(st.MaxActive, 1) {
				allowDown = false
				m.queued = true
			} else {
				m.queued = false
				active++
			}
		}
		if allowDown && !complete {
			m.t.AllowDataDownload()
		} else {
			m.t.DisallowDataDownload()
		}

		if complete {
			if m.row.DoneAt == 0 {
				m.row.DoneAt = time.Now().Unix()
				m.seedFrom = time.Now()
				_ = e.Store.SaveTorrent(m.row)
			}
			if m.seedFrom.IsZero() {
				m.seedFrom = time.Now()
			}
			if !m.row.Imported && st.AutoImport && (m.row.Mode == "download" || m.row.Keep) {
				m.row.Imported = true
				_ = e.Store.SaveTorrent(m.row)
				go e.importTorrent(m)
			}
			ratio := 0.0
			if total := m.t.Length(); total > 0 {
				ratio = float64(written) / float64(total)
			}
			limitHit := st.SeedRatio <= 0 || ratio >= st.SeedRatio || st.SeedMinutes > 0 && time.Since(m.seedFrom) > time.Duration(st.SeedMinutes)*time.Minute
			if limitHit || m.row.Paused {
				m.t.DisallowDataUpload()
				m.stopped = limitHit
			} else {
				m.t.AllowDataUpload()
			}
		} else if m.row.Paused {
			m.t.DisallowDataUpload()
		} else {
			m.t.AllowDataUpload()
		}
	}
}

func smooth(prev, cur float64) float64 { return prev*0.6 + cur*0.4 }

func wantedComplete(m *managed) bool {
	off := map[int]bool{}
	for _, i := range m.row.FilesOff {
		off[i] = true
	}
	for i, f := range m.t.Files() {
		if off[i] || Blocked(f.Path()) {
			continue
		}
		if f.BytesCompleted() < f.Length() {
			return false
		}
	}
	return true
}

// importTorrent places finished video files into the library. Files are
// hard-linked when possible so seeding continues from the download folder.
func (e *Engine) importTorrent(m *managed) {
	st := e.Settings()
	movies, shows := st.MoviesDir, st.ShowsDir
	if movies == "" {
		movies = filepath.Join(st.DownloadDir, "Фильмы")
	}
	if shows == "" {
		shows = filepath.Join(st.DownloadDir, "Сериалы")
	}
	for _, f := range m.t.Files() {
		if !parse.IsVideo(f.Path()) || parse.IsSample(f.Path()) || Blocked(f.Path()) || f.BytesCompleted() < f.Length() {
			continue
		}
		src := filepath.Join(st.DownloadDir, filepath.FromSlash(f.Path()))
		dst, err := placeFile(src, movies, shows)
		root, kind := movies, "movies"
		if rel, rerr := filepath.Rel(shows, dst); rerr == nil && !strings.HasPrefix(rel, "..") {
			root, kind = shows, "shows"
		}
		if err != nil {
			log.Printf("import %s: %v", src, err)
			e.mu.Lock()
			m.err = "импорт: " + err.Error()
			e.mu.Unlock()
			continue
		}
		if e.Importer != nil {
			if err := e.Importer.ImportFile(context.Background(), dst, root, kind); err != nil {
				log.Printf("import index %s: %v", dst, err)
			}
		}
		e.mu.Lock()
		m.imported = append(m.imported, dst)
		e.mu.Unlock()
	}
}

// ---------- listing ----------

type FileInfo struct {
	Index    int     `json:"index"`
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Wanted   bool    `json:"wanted"`
	Video    bool    `json:"video"`
	Blocked  bool    `json:"blocked"`
}

type Info struct {
	Hash      string     `json:"hash"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	Done      int64      `json:"done"`
	Progress  float64    `json:"progress"`
	Status    string     `json:"status"` // meta | downloading | streaming | queued | paused | seeding | done | error | stopped
	Mode      string     `json:"mode"`
	Keep      bool       `json:"keep"`
	Down      float64    `json:"down"` // bytes/s
	Up        float64    `json:"up"`
	Peers     int        `json:"peers"`
	Seeds     int        `json:"seeds"`
	Ratio     float64    `json:"ratio"`
	ETA       int64      `json:"eta"` // seconds, -1 unknown
	Priority  int        `json:"priority"`
	AddedAt   int64      `json:"addedAt"`
	DoneAt    int64      `json:"doneAt"`
	Files     []FileInfo `json:"files,omitempty"`
	Imported  []string   `json:"imported,omitempty"`
	Error     string     `json:"error,omitempty"`
	FileCount int        `json:"fileCount"`
}

type Overview struct {
	Running bool    `json:"running"`
	Error   string  `json:"error,omitempty"`
	VPNDown bool    `json:"vpnDown"`
	DiskLow bool    `json:"diskLow"`
	FreeGB  float64 `json:"freeGB"`
	Down    float64 `json:"down"`
	Up      float64 `json:"up"`
	Iface   string  `json:"iface"`
	Kill    bool    `json:"killSwitch"`
	Port    int     `json:"port"`
	Items   []Info  `json:"items"`
}

func (e *Engine) List() Overview {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.settings
	ov := Overview{Running: e.cl != nil, Error: e.startErr, VPNDown: e.vpnDown, DiskLow: e.diskLow, Iface: st.BindIface, Kill: st.KillSwitch, Port: st.Port, Items: []Info{}}
	if free, err := FreeSpace(st.DownloadDir); err == nil {
		ov.FreeGB = float64(free) / (1 << 30)
	}
	for _, m := range e.items {
		info := e.infoLocked(m, false)
		ov.Down += info.Down
		ov.Up += info.Up
		ov.Items = append(ov.Items, info)
	}
	if !ov.Running {
		// Show stored torrents even while the engine is stopped (VPN down).
		rows, _ := e.Store.Torrents()
		for _, r := range rows {
			ov.Items = append(ov.Items, Info{Hash: r.InfoHash, Name: r.Name, Status: "paused", Mode: r.Mode, AddedAt: r.AddedAt, DoneAt: r.DoneAt, ETA: -1})
		}
	}
	sort.Slice(ov.Items, func(i, j int) bool {
		if ov.Items[i].Priority != ov.Items[j].Priority {
			return ov.Items[i].Priority > ov.Items[j].Priority
		}
		return ov.Items[i].AddedAt > ov.Items[j].AddedAt
	})
	return ov
}

func (e *Engine) Get(hash string) (Info, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.get(hash)
	if err != nil {
		return Info{}, err
	}
	return e.infoLocked(m, true), nil
}

func (e *Engine) infoLocked(m *managed, withFiles bool) Info {
	t := m.t
	info := Info{Hash: m.row.InfoHash, Name: m.row.Name, Mode: m.row.Mode, Keep: m.row.Keep, Priority: m.row.Priority, AddedAt: m.row.AddedAt, DoneAt: m.row.DoneAt,
		Down: m.down, Up: m.up, ETA: -1, Error: m.err, Imported: m.imported}
	s := t.Stats()
	info.Peers = s.ActivePeers
	info.Seeds = s.ConnectedSeeders
	if t.Info() == nil {
		info.Status = "meta"
		if m.row.Paused {
			info.Status = "paused"
		}
		return info
	}
	info.Name = t.Name()
	off := map[int]bool{}
	for _, i := range m.row.FilesOff {
		off[i] = true
	}
	for i, f := range t.Files() {
		wanted := !off[i] && !Blocked(f.Path())
		info.FileCount++
		if wanted {
			info.Size += f.Length()
			info.Done += f.BytesCompleted()
		}
		if withFiles {
			fi := FileInfo{Index: i, Path: f.DisplayPath(), Size: f.Length(), Wanted: wanted, Video: parse.IsVideo(f.Path()), Blocked: Blocked(f.Path())}
			if f.Length() > 0 {
				fi.Progress = float64(f.BytesCompleted()) / float64(f.Length())
			}
			info.Files = append(info.Files, fi)
		}
	}
	if info.Size > 0 {
		info.Progress = float64(info.Done) / float64(info.Size)
	}
	if total := t.Length(); total > 0 {
		info.Ratio = float64(s.BytesWrittenData.Int64()) / float64(total)
	}
	if m.down > 1024 && info.Size > info.Done {
		info.ETA = int64(float64(info.Size-info.Done) / m.down)
	}
	switch {
	case m.row.Paused:
		info.Status = "paused"
	case len(m.streams) > 0:
		info.Status = "streaming"
	case info.Progress >= 1 && m.stopped:
		info.Status = "done"
	case info.Progress >= 1:
		info.Status = "seeding"
	case e.diskLow:
		info.Status = "nospace"
	case m.queued && !inSchedule(e.settings, time.Now()):
		info.Status = "scheduled"
	case m.queued:
		info.Status = "queued"
	case m.row.Mode == "stream":
		info.Status = "streaming"
	default:
		info.Status = "downloading"
	}
	return info
}

// ---------- helpers ----------

func ifaceIPv4(name string) (net.IP, error) {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	if ifc.Flags&net.FlagUp == 0 {
		return nil, errors.New("интерфейс выключен")
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP.To4(), nil
		}
	}
	return nil, errors.New("нет IPv4-адреса")
}

type IfaceInfo struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Up   bool   `json:"up"`
	VPN  bool   `json:"vpn"`
}

// Interfaces lists network interfaces for the "bind to interface" setting.
func Interfaces() []IfaceInfo {
	var out []IfaceInfo
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		info := IfaceInfo{Name: i.Name, Up: i.Flags&net.FlagUp != 0}
		if ip, err := ifaceIPv4(i.Name); err == nil {
			info.IP = ip.String()
		}
		n := strings.ToLower(i.Name)
		info.VPN = strings.HasPrefix(n, "wg") || strings.HasPrefix(n, "tun") || strings.HasPrefix(n, "tap") || strings.Contains(n, "vpn") ||
			strings.Contains(n, "wireguard") || strings.Contains(n, "openvpn") || strings.Contains(n, "proton") || strings.Contains(n, "nord") || i.Flags&net.FlagPointToPoint != 0
		out = append(out, info)
	}
	return out
}
