package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/netutil"
	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/renderer"
	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/transcode"
)

// browserCaps: what the PC player can decode. The UI reports optional codecs
// it detected with MediaSource.isTypeSupported (hevc, ac3, hdr), whether it
// opens Matroska (mkv) and whether direct play of this file already failed
// (nodirect).
func browserCaps(r *http.Request) renderer.Caps {
	q := r.URL.Query()
	c := renderer.Caps{
		Video:     []string{"h264", "vp9", "av1", "vp8"},
		Audio:     []string{"aac", "mp3", "opus", "vorbis", "flac"},
		Container: []string{"mp4", "webm", "mov", "m4v"},
		MaxHeight: 4320, TextSubs: true, ToneMaps: true,
	}
	if q.Get("mkv") == "1" {
		c.Container = append(c.Container, "mkv")
	}
	c.NoDirect = q.Get("nodirect") == "1"
	if q.Get("hevc") == "1" {
		c.Video = append(c.Video, "hevc")
	}
	if q.Get("ac3") == "1" {
		c.Audio = append(c.Audio, "ac3", "eac3")
	}
	c.HDR = q.Get("hdr") == "1"
	return c
}

type decision struct {
	Direct    bool
	CopyVideo bool
	CopyAudio bool
	Tonemap   bool
	Reason    string
	Summary   string // e.g. "HEVC 10-bit → H.264 1080p"
}

func (s *Server) decide(f *store.File, audio *store.Track, caps renderer.Caps, quality string, firstAudio bool) decision {
	d := decision{}
	target := map[string]int{"1080p": 1080, "720p": 720, "480p": 480}[quality]
	downscale := target > 0 && f.Height > target+40
	vOK := caps.Has(caps.Video, f.VideoCodec) && !(f.VideoCodec == "h264" && f.BitDepth > 8) &&
		hdrOK(f.HDR, caps) && f.Height <= caps.MaxHeight+40 && !downscale
	aOK := audio == nil || caps.Has(caps.Audio, audio.Codec)
	cOK := caps.Has(caps.Container, f.Container)
	var why []string
	if !vOK {
		switch {
		case !hdrOK(f.HDR, caps):
			why = append(why, "HDR не поддерживается экраном")
		case downscale:
			why = append(why, "выбрано качество "+quality)
		default:
			why = append(why, "видеокодек "+strings.ToUpper(f.VideoCodec))
		}
	}
	if !aOK {
		why = append(why, "аудио "+strings.ToUpper(audio.Codec))
	}
	if !cOK {
		why = append(why, "контейнер "+f.Container)
	}
	if caps.NoDirect {
		why = append(why, "файл не открылся напрямую")
	}
	if vOK && aOK && cOK && firstAudio && !caps.NoDirect {
		d.Direct = true
		d.Summary = "Напрямую"
		return d
	}
	// HLS in MPEG-TS carries H.264/HEVC video and AAC/MP3 audio as is.
	d.CopyVideo = vOK && (f.VideoCodec == "h264" || f.VideoCodec == "hevc")
	d.CopyAudio = audio != nil && aOK && (audio.Codec == "aac" || audio.Codec == "mp3")
	d.Tonemap = !d.CopyVideo && f.HDR != "" && !caps.HDR && s.Store.Setting("tonemap", "1") == "1" && s.Trans.HW().Zscale
	if len(why) == 0 && !firstAudio {
		why = append(why, "выбрана другая аудиодорожка")
	}
	d.Reason = strings.Join(why, ", ")
	switch {
	case d.CopyVideo && d.CopyAudio:
		d.Summary = "Перепаковка без перекодирования"
	case d.CopyVideo:
		d.Summary = "Видео без изменений, звук → AAC"
	default:
		out := "H.264"
		if target > 0 && f.Height > target {
			out += fmt.Sprintf(" %dp", target)
		} else if f.Height > 0 {
			out += fmt.Sprintf(" %dp", min(f.Height, 2160))
		}
		src := strings.ToUpper(f.VideoCodec)
		if f.BitDepth > 8 {
			src += fmt.Sprintf(" %d-bit", f.BitDepth)
		}
		d.Summary = src + " → " + out
	}
	return d
}

// hdrOK: HDR video plays as is on an HDR screen, or on an SDR screen when
// the player tone-maps by itself. Dolby Vision without an HDR10/HLG base
// layer (profile 5) is left to HDR TVs, which usually decode DV; browsers
// would show it in wrong colours.
func hdrOK(hdr string, caps renderer.Caps) bool {
	switch {
	case hdr == "":
		return true
	case hdr == "DV":
		return caps.HDR && !caps.ToneMaps
	}
	return caps.HDR || caps.ToneMaps
}

type compatRow struct {
	Name   string `json:"name"`
	Mode   string `json:"mode"`
	Status string `json:"status"` // direct | transcode
}

// compat answers "where will it play" on the movie page.
func (s *Server) compat(it *store.Item) []compatRow {
	if len(it.Files) == 0 {
		return nil
	}
	f := &it.Files[0]
	var audio *store.Track
	if len(f.Audio) > 0 {
		audio = &f.Audio[0]
	}
	quality := s.Store.Setting("quality", "original")
	pc := renderer.Caps{Video: []string{"h264", "vp9", "av1", "hevc"}, Audio: []string{"aac", "mp3", "opus", "flac"}, Container: []string{"mp4", "webm", "mov", "m4v", "mkv"}, MaxHeight: 4320, ToneMaps: true}
	rows := []compatRow{}
	d := s.decide(f, audio, pc, quality, true)
	rows = append(rows, compatRow{"Этот ПК", d.Summary, statusOf(d)})
	for _, dev := range s.Cast.Devices() {
		dd := s.decide(f, audio, dev.Caps, quality, true)
		mode := dd.Summary
		if !dd.Direct && dd.CopyVideo {
			mode = "Почти напрямую: " + strings.ToLower(dd.Summary)
		} else if !dd.Direct {
			mode = "Транскод: " + dd.Summary
		}
		rows = append(rows, compatRow{dev.Name, mode, statusOf(dd)})
	}
	return rows
}

func statusOf(d decision) string {
	if d.Direct || d.CopyVideo {
		return "direct"
	}
	return "transcode"
}

type trackOut struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Codec    string `json:"codec"`
	Lang     string `json:"lang"`
	Selected bool   `json:"selected"`
	URL      string `json:"url,omitempty"`
	Bitmap   bool   `json:"bitmap,omitempty"`
	External bool   `json:"external,omitempty"`
}

type playback struct {
	Mode     string     `json:"mode"` // direct | hls
	URL      string     `json:"url"`
	Offset   float64    `json:"offset"` // the stream starts at this position of the movie
	Start    float64    `json:"start"`  // seek to this position after loading
	Duration float64    `json:"duration"`
	Session  string     `json:"session,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	Summary  string     `json:"summary"`
	Encoder  string     `json:"encoder,omitempty"`
	Audio    []trackOut `json:"audio"`
	Subs     []trackOut `json:"subs"`
	FileID   int64      `json:"fileId"`
	ItemID   int64      `json:"itemId"`
	Title    string     `json:"title"`
	Info     string     `json:"info"`
	External bool       `json:"external"`
}

func langLabel(t store.Track, i int, kind string) string {
	names := map[string]string{"rus": "Русский", "ru": "Русский", "eng": "English", "en": "English", "ukr": "Українська", "jpn": "日本語", "ger": "Deutsch", "deu": "Deutsch", "fre": "Français", "fra": "Français", "spa": "Español", "ita": "Italiano", "kor": "한국어", "chi": "中文", "zho": "中文"}
	l := names[strings.ToLower(t.Lang)]
	if l == "" && t.Lang != "" && t.Lang != "und" {
		l = strings.ToUpper(t.Lang)
	}
	if l == "" {
		l = fmt.Sprintf("%s %d", kind, i+1)
	}
	if t.Title != "" && !strings.EqualFold(t.Title, l) {
		l += " — " + t.Title
	}
	return l
}

func audioMeta(t store.Track) string {
	ch := ""
	switch t.Channels {
	case 1:
		ch = " 1.0"
	case 2:
		ch = " 2.0"
	case 6:
		ch = " 5.1"
	case 8:
		ch = " 7.1"
	}
	c := strings.ToUpper(t.Codec)
	if c == "EAC3" {
		c = "DD+"
	} else if c == "AC3" {
		c = "DD"
	}
	return c + ch
}

// playInfo decides direct play vs HLS for one file and returns URLs (FR-8).
func (s *Server) playInfo(w http.ResponseWriter, r *http.Request) error {
	fileID, err := pathID(r, "fileId")
	if err != nil {
		return err
	}
	f, err := s.Store.FileByID(fileID)
	if err != nil {
		return notFound("файл не найден")
	}
	if _, err := os.Stat(f.Path); err != nil {
		return notFound("файл недоступен — диск отключён?")
	}
	pid := s.profileID(r)
	it, _ := s.Store.Item(pid, f.ItemID)
	pb, err := s.preparePlayback(r.Context(), f, f.Path, browserCaps(r), r.URL.Query(), "", "")
	if err != nil {
		return err
	}
	pb.ItemID = f.ItemID
	if it != nil {
		pb.Title = it.Title
		if it.Kind == "episode" {
			pb.Title = fmt.Sprintf("%s · S%dE%d", it.ShowTitle, it.Season, it.Episode)
			if it.EpTitle != "" {
				pb.Title += " · " + it.EpTitle
			}
		}
		pb.Info = fileInfoLine(it.Year, f, pb.Mode)
	}
	pb.External = s.Store.Setting("useExternal", "") == "1"
	return writeJSON(w, pb)
}

func fileInfoLine(year int, f *store.File, mode string) string {
	var parts []string
	if year > 0 {
		parts = append(parts, strconv.Itoa(year))
	}
	if q := store.QualityLabel(f.Width, f.HDR); q != "" {
		parts = append(parts, q)
	}
	if mode == "direct" {
		parts = append(parts, "Напрямую")
	} else {
		parts = append(parts, "Транскод")
	}
	return strings.Join(parts, " · ")
}

// preparePlayback is shared by the PC player, TVs and torrent streams.
// base is "" for same-origin URLs or "http://ip:port/api/t/<token>" for TVs.
func (s *Server) preparePlayback(ctx context.Context, f *store.File, input string, caps renderer.Caps, q map[string][]string, base, directURL string) (*playback, error) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	start, _ := strconv.ParseFloat(get("start"), 64)
	quality := get("quality")
	if quality == "" {
		quality = s.Store.Setting("quality", "original")
	}
	audioIdx := -1
	if v, err := strconv.Atoi(get("audio")); err == nil {
		audioIdx = v
	}
	subIdx := -1
	if v, err := strconv.Atoi(get("sub")); err == nil {
		subIdx = v
	}
	burn := get("burn") == "1"

	pb := &playback{FileID: f.ID, Duration: f.Duration, Audio: []trackOut{}, Subs: []trackOut{}}
	var audio *store.Track
	for i, a := range f.Audio {
		sel := a.Index == audioIdx || audioIdx < 0 && i == 0
		if sel {
			a := a
			audio = &a
		}
		pb.Audio = append(pb.Audio, trackOut{Index: a.Index, Label: langLabel(a, i, "Дорожка") + " · " + audioMeta(a), Codec: a.Codec, Lang: a.Lang, Selected: sel})
	}
	if audio == nil && len(f.Audio) > 0 {
		audio = &f.Audio[0]
		pb.Audio[0].Selected = true
	}
	firstAudio := audio == nil || audio.Index == f.Audio[0].Index

	d := s.decide(f, audio, caps, quality, firstAudio)
	var burnSub *store.Track
	burnPos := 0
	if burn && subIdx >= 0 && subIdx < len(f.Subs) {
		t := f.Subs[subIdx]
		burnSub = &t
		for i := 0; i < subIdx; i++ {
			if f.Subs[i].External == "" {
				burnPos++
			}
		}
		d.Direct, d.CopyVideo = false, false
		d.Summary += ", субтитры вжигаются"
	}
	pb.Summary, pb.Reason = d.Summary, d.Reason

	if d.Direct {
		pb.Mode = "direct"
		pb.Start = start
		pb.URL = directURL
		if pb.URL == "" {
			pb.URL = base + fmt.Sprintf("/api/files/%d/direct", f.ID)
			if base != "" {
				pb.URL = base + fmt.Sprintf("/files/%d/direct", f.ID)
			}
		}
	} else {
		height := f.Height
		if d.CopyVideo && start > 0 {
			// A stream copy can only start on a keyframe: start the session (and
			// the player's clock) there, so audio and subtitles stay in sync.
			if kf, ok := s.Trans.KeyframeBefore(ctx, input, start); ok {
				start = kf
			}
		}
		sess, err := s.Trans.Start(transcode.Options{
			FileID: f.ID, Input: input, Start: start, Audio: audioIndexOr(audio), Quality: quality,
			CopyVideo: d.CopyVideo, CopyAudio: d.CopyAudio, Tonemap: d.Tonemap, BurnSub: burnSub, BurnSubPos: burnPos, Height: height,
		})
		if err != nil {
			return nil, httpError{http.StatusServiceUnavailable, err.Error()}
		}
		s.Trans.StopFile(f.ID, sess.ID) // a seek replaces the previous session
		pb.Mode = "hls"
		pb.Offset = start
		pb.Session = sess.ID
		if base == "" {
			pb.URL = "/api/hls/" + sess.ID + "/index.m3u8"
		} else {
			pb.URL = base + "/hls/" + sess.ID + "/index.m3u8"
		}
		if !d.CopyVideo {
			pb.Encoder = s.Trans.Encoder()
		}
	}
	for i, t := range f.Subs {
		to := trackOut{Index: i, Label: langLabel(t, i, "Субтитры"), Codec: t.Codec, Lang: t.Lang, Selected: i == subIdx, Bitmap: transcode.IsBitmapSub(t.Codec), External: t.External != ""}
		if t.External != "" {
			to.Label += " · " + strings.ToUpper(t.Codec) + " · внешние"
		} else {
			to.Label += " · " + strings.ToUpper(strings.TrimSuffix(t.Codec, "_subtitle")) + " · встроенные"
		}
		if !to.Bitmap {
			to.URL = fmt.Sprintf("/api/files/%d/subs/%d", f.ID, i)
			if pb.Mode == "hls" && pb.Offset > 0 {
				to.URL += "?offset=" + strconv.FormatFloat(pb.Offset, 'f', 3, 64)
			}
		}
		pb.Subs = append(pb.Subs, to)
	}
	return pb, nil
}

func audioIndexOr(t *store.Track) int {
	if t == nil {
		return -1
	}
	return t.Index
}

// direct serves the file itself with Range support (FR-8 direct play).
func (s *Server) direct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	f, err := s.Store.FileByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		http.Error(w, "файл недоступен", http.StatusNotFound)
		return
	}
	defer fh.Close()
	st, _ := fh.Stat()
	ct := map[string]string{"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime", "webm": "video/webm", "mkv": "video/x-matroska", "avi": "video/x-msvideo", "ts": "video/mp2t"}[f.Container]
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	// The file name is not sent: paths never leave the server.
	http.ServeContent(w, r, "", st.ModTime(), fh)
}

func (s *Server) hls(w http.ResponseWriter, r *http.Request) {
	sess := s.Trans.Get(r.PathValue("session"))
	if sess == nil {
		http.Error(w, "сессия завершена", http.StatusGone)
		return
	}
	name := filepath.Base(r.PathValue("file"))
	p, err := sess.WaitFile(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if strings.HasSuffix(name, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // Cast receivers fetch cross-origin
	http.ServeFile(w, r, p)
}

func (s *Server) stopHLS(w http.ResponseWriter, r *http.Request) error {
	s.Trans.Stop(r.PathValue("session"))
	return writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) subtitle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, _ := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".vtt"))
	f, err := s.Store.FileByID(id)
	if err != nil || n < 0 || n >= len(f.Subs) {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Has("from") {
		s.serveSubtitleFrom(w, r, f.Path, f.Path, f.Subs[n])
		return
	}
	p, err := s.Trans.Subtitle(r.Context(), f.Path, f.Subs[n], queryFloat(r, "offset"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, p)
}

// serveSubtitleFrom answers the PC player, which asks for cues from its
// position on and polls while X-Subs-Done is 0: cues arrive as ffmpeg reads
// through the file instead of after it has read all of it.
func (s *Server) serveSubtitleFrom(w http.ResponseWriter, r *http.Request, source, input string, t store.Track) {
	p, done, err := s.Trans.SubtitleFrom(source, input, t, queryFloat(r, "from"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	b, _ := os.ReadFile(p)
	if !done {
		// Only whole cues: the last one may still be half written.
		if i := bytes.LastIndex(b, []byte("\n\n")); i >= 0 {
			b = b[:i+2]
		} else {
			b = nil
		}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Subs-Done", map[bool]string{true: "1", false: "0"}[done])
	_, _ = w.Write(b)
}

// openExternal launches VLC/mpv on this PC (FR-9).
func (s *Server) openExternal(w http.ResponseWriter, r *http.Request) error {
	if !netutil.IsLoopback(r) {
		return httpError{http.StatusForbidden, "внешний плеер запускается только на этом ПК"}
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	f, err := s.Store.FileByID(id)
	if err != nil {
		return notFound("файл не найден")
	}
	player := s.Store.Setting("externalPlayer", "")
	if player == "" {
		player = findPlayer()
	}
	if player == "" {
		return badRequest("VLC или mpv не найдены — укажите путь в настройках")
	}
	args := []string{f.Path}
	if start := queryFloat(r, "start"); start > 0 {
		if strings.Contains(strings.ToLower(player), "mpv") {
			args = append([]string{fmt.Sprintf("--start=%.0f", start)}, args...)
		} else {
			args = append([]string{fmt.Sprintf("--start-time=%.0f", start)}, args...)
		}
	}
	cmd := exec.Command(player, args...)
	if err := cmd.Start(); err != nil {
		return badRequest("не удалось запустить %s: %v", player, err)
	}
	go func() { _ = cmd.Wait() }()
	return writeJSON(w, map[string]bool{"ok": true})
}

func findPlayer() string {
	cands := []string{"vlc", "mpv"}
	for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if pf != "" {
			cands = append([]string{filepath.Join(pf, "VideoLAN", "VLC", "vlc.exe"), filepath.Join(pf, "mpv", "mpv.exe")}, cands...)
		}
	}
	for _, c := range cands {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// ---------- probing remote inputs (torrent streams) ----------

type probeCache struct {
	mu sync.Mutex
	m  map[string]*store.File
}

func newProbeCache() *probeCache { return &probeCache{m: map[string]*store.File{}} }

func (s *Server) probeURL(ctx context.Context, key, url string) (*store.File, error) {
	s.probes.mu.Lock()
	if f, ok := s.probes.m[key]; ok {
		s.probes.mu.Unlock()
		return f, nil
	}
	s.probes.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	p := &probe.Prober{FFprobe: s.Cfg.FFprobe}
	f, err := p.ProbeURL(ctx, url)
	if err != nil {
		return nil, errors.New("не удалось прочитать начало файла: " + err.Error())
	}
	s.probes.mu.Lock()
	s.probes.m[key] = &f
	s.probes.mu.Unlock()
	return &f, nil
}
