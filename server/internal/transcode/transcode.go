// Package transcode runs ffmpeg: on-the-fly HLS for clients that cannot play
// a file directly (FR-8, FR-14), hardware encoders with CPU fallback (FR-16),
// quality profiles, a stream limit and a self-cleaning cache (FR-17), HDR→SDR
// tonemapping (FR-18) and subtitle conversion to WebVTT.
package transcode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

type Options struct {
	FileID     int64
	Input      string  // path or http URL (torrent stream)
	Start      float64 // seconds
	Audio      int     // ffmpeg stream index, -1 = first audio
	Quality    string  // original | 1080p | 720p
	CopyVideo  bool    // video is compatible: remux only
	CopyAudio  bool
	Tonemap    bool
	BurnSub    *store.Track // subtitle burned into the picture (TV without subtitle support)
	BurnSubPos int          // position of an embedded BurnSub among subtitle streams
	Height     int          // source height, to skip pointless downscales
}

type Session struct {
	ID       string
	Dir      string
	Opts     Options
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	started  time.Time
	mu       sync.Mutex
	lastSeen time.Time
	done     chan struct{}
	err      error
	Speed    float64 // encoding speed, x realtime
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

func (s *Session) idle() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastSeen)
}

type Manager struct {
	FFmpeg     string
	CacheDir   string
	MaxStreams func() int
	HWSetting  func() string // auto | nvenc | qsv | amf | vaapi | cpu
	TonemapOn  func() bool

	mu       sync.Mutex
	sessions map[string]*Session
	hw       *HWInfo
	hwOnce   sync.Once
}

func New(ffmpeg, cacheDir string) *Manager {
	m := &Manager{FFmpeg: ffmpeg, CacheDir: cacheDir, sessions: map[string]*Session{},
		MaxStreams: func() int { return 2 }, HWSetting: func() string { return "auto" }, TonemapOn: func() bool { return true }}
	_ = os.RemoveAll(filepath.Join(cacheDir, "hls")) // leftovers from a previous run
	_ = os.MkdirAll(filepath.Join(cacheDir, "hls"), 0o755)
	_ = os.MkdirAll(filepath.Join(cacheDir, "subs"), 0o755)
	return m
}

// ---------- hardware detection ----------

type HWInfo struct {
	FFmpegFound bool     `json:"ffmpegFound"`
	Version     string   `json:"version"`
	Encoders    []string `json:"encoders"` // working hardware H.264 encoders
	Zscale      bool     `json:"zscale"`   // HDR tonemapping available
}

var hwEncoders = []struct{ key, encoder string }{
	{"nvenc", "h264_nvenc"}, {"qsv", "h264_qsv"}, {"amf", "h264_amf"}, {"vaapi", "h264_vaapi"},
}

func (m *Manager) HW() *HWInfo {
	m.hwOnce.Do(func() { m.hw = m.detect() })
	return m.hw
}

func (m *Manager) detect() *HWInfo {
	info := &HWInfo{}
	out, err := m.run(10*time.Second, "-hide_banner", "-version")
	if err != nil {
		return info
	}
	info.FFmpegFound = true
	if line, _, ok := strings.Cut(string(out), "\n"); ok {
		info.Version = strings.TrimSpace(strings.TrimPrefix(line, "ffmpeg version "))
		if i := strings.Index(info.Version, " Copyright"); i > 0 {
			info.Version = info.Version[:i]
		}
	}
	filters, _ := m.run(10*time.Second, "-hide_banner", "-filters")
	info.Zscale = bytes.Contains(filters, []byte(" zscale "))
	encs, _ := m.run(10*time.Second, "-hide_banner", "-encoders")
	for _, e := range hwEncoders {
		if !bytes.Contains(encs, []byte(e.encoder)) {
			continue
		}
		// The encoder being compiled in is not enough: try a tiny encode.
		args := []string{"-hide_banner", "-v", "error"}
		if e.key == "vaapi" {
			args = append(args, "-vaapi_device", "/dev/dri/renderD128")
		}
		args = append(args, "-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.2")
		if e.key == "vaapi" {
			args = append(args, "-vf", "format=nv12,hwupload")
		}
		args = append(args, "-c:v", e.encoder, "-f", "null", "-")
		if _, err := m.run(15*time.Second, args...); err == nil {
			info.Encoders = append(info.Encoders, e.key)
		}
	}
	log.Printf("ffmpeg %s, hw encoders: %v", info.Version, info.Encoders)
	return info
}

func (m *Manager) run(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	probe.HideWindow(cmd)
	return cmd.CombinedOutput()
}

// encoder picks the configured hardware encoder, falling back to libx264.
func (m *Manager) encoder() string {
	want := m.HWSetting()
	if want == "cpu" {
		return "cpu"
	}
	for _, e := range m.HW().Encoders {
		if want == "auto" || want == e {
			return e
		}
	}
	return "cpu"
}

// ---------- HLS sessions ----------

func sessionKey(o Options) string {
	burn := ""
	if o.BurnSub != nil {
		burn = fmt.Sprintf("%d%s", o.BurnSub.Index, o.BurnSub.External)
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%d|%s|%.0f|%d|%s|%v|%v|%v|%s", o.FileID, o.Input, o.Start, o.Audio, o.Quality, o.CopyVideo, o.CopyAudio, o.Tonemap, burn)))
	return hex.EncodeToString(h[:8])
}

// Start returns a running (or newly started) HLS session for the options.
func (m *Manager) Start(o Options) (*Session, error) {
	if !m.HW().FFmpegFound {
		return nil, errors.New("ffmpeg не найден")
	}
	id := sessionKey(o)
	m.mu.Lock()
	if s, ok := m.sessions[id]; ok {
		m.mu.Unlock()
		s.touch()
		return s, nil
	}
	// Enforce the parallel stream limit by stopping the least recently used.
	for len(m.sessions) >= max(m.MaxStreams(), 1) {
		var oldest *Session
		for _, s := range m.sessions {
			if oldest == nil || s.idle() > oldest.idle() {
				oldest = s
			}
		}
		if oldest.idle() < 15*time.Second {
			m.mu.Unlock()
			return nil, fmt.Errorf("достигнут лимит одновременных транскодов (%d)", m.MaxStreams())
		}
		m.stopLocked(oldest.ID)
	}
	dir := filepath.Join(m.CacheDir, "hls", id)
	_ = os.MkdirAll(dir, 0o755)
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{ID: id, Dir: dir, Opts: o, cancel: cancel, started: time.Now(), lastSeen: time.Now(), done: make(chan struct{})}
	args := m.buildArgs(o, dir)
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	probe.HideWindow(cmd)
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		cancel()
		return nil, err
	}
	s.cmd = cmd
	m.sessions[id] = s
	m.mu.Unlock()
	go s.readProgress(stderr)
	go func() {
		s.err = cmd.Wait()
		close(s.done)
		if s.err != nil && ctx.Err() == nil {
			log.Printf("ffmpeg session %s failed: %v", id, s.err)
		}
	}()
	return s, nil
}

var reSpeed = regexp.MustCompile(`speed=\s*([\d.]+)x`)

func (s *Session) readProgress(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Split(scanCRLF)
	var tail []string
	for sc.Scan() {
		line := sc.Text()
		if m := reSpeed.FindStringSubmatch(line); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			s.mu.Lock()
			s.Speed = v
			s.mu.Unlock()
		} else if strings.TrimSpace(line) != "" {
			tail = append(tail, line)
			if len(tail) > 8 {
				tail = tail[1:]
			}
		}
	}
	if len(tail) > 0 {
		s.mu.Lock()
		if s.err == nil {
			s.err = errors.New(strings.Join(tail, "\n"))
		}
		s.mu.Unlock()
	}
}

func scanCRLF(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (m *Manager) buildArgs(o Options, dir string) []string {
	enc := m.encoder()
	args := []string{"-hide_banner", "-v", "warning", "-stats", "-nostdin", "-y"}
	if strings.HasPrefix(o.Input, "http") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
	}
	if !o.CopyVideo && enc == "vaapi" {
		args = append(args, "-vaapi_device", "/dev/dri/renderD128")
	}
	if o.Start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(o.Start, 'f', 3, 64))
		if o.CopyVideo {
			// Copied video starts on the keyframe before Start; without this the
			// audio would start seconds later and play out of sync.
			args = append(args, "-noaccurate_seek")
		}
	}
	args = append(args, "-i", o.Input, "-map", "0:v:0")
	if o.Audio >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", o.Audio))
	} else {
		args = append(args, "-map", "0:a:0?")
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1", "-max_muxing_queue_size", "2048")

	if o.CopyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		var vf []string
		if o.Tonemap {
			vf = append(vf, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709", "tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv")
		}
		target := map[string]int{"1080p": 1080, "720p": 720, "480p": 480}[o.Quality]
		if target > 0 && (o.Height == 0 || o.Height > target) {
			vf = append(vf, fmt.Sprintf("scale=-2:%d", target))
		}
		if o.BurnSub != nil {
			vf = append(vf, burnFilter(o))
		}
		vf = append(vf, "format=yuv420p")
		if enc == "vaapi" {
			vf = append(vf, "format=nv12", "hwupload")
		}
		args = append(args, "-vf", strings.Join(vf, ","))
		bitrate := map[string]string{"1080p": "8M", "720p": "4M", "480p": "2M"}[o.Quality]
		if bitrate == "" {
			bitrate = "12M"
		}
		switch enc {
		case "nvenc":
			args = append(args, "-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-b:v", bitrate, "-maxrate", bitrate)
		case "qsv":
			args = append(args, "-c:v", "h264_qsv", "-preset", "faster", "-b:v", bitrate, "-maxrate", bitrate)
		case "amf":
			args = append(args, "-c:v", "h264_amf", "-quality", "speed", "-b:v", bitrate, "-maxrate", bitrate)
		case "vaapi":
			args = append(args, "-c:v", "h264_vaapi", "-b:v", bitrate, "-maxrate", bitrate)
		default:
			args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-maxrate", bitrate, "-bufsize", "2M")
		}
		args = append(args, "-profile:v", "high", "-g", "96", "-keyint_min", "96", "-sc_threshold", "0", "-force_key_frames", "expr:gte(t,n_forced*4)")
	}
	if o.CopyAudio {
		args = append(args, "-c:a", "copy")
	} else {
		// async resampling fills gaps and drops overlaps after a seek or a
		// damaged spot in a partly downloaded file instead of garbling sound.
		args = append(args, "-af", "aresample=async=1000", "-c:a", "aac", "-ac", "2", "-b:a", "192k")
	}
	args = append(args,
		"-f", "hls", "-hls_time", "4", "-hls_list_size", "0", "-hls_playlist_type", "event",
		"-hls_flags", "independent_segments+temp_file",
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"),
		filepath.Join(dir, "index.m3u8"))
	return args
}

var reFrame = regexp.MustCompile(`(?m)^0,\s*(-?\d+),`)
var reTimebase = regexp.MustCompile(`(?m)^#tb 0: (\d+)/(\d+)`)

// KeyframeBefore returns the position of the video keyframe that a seek to t
// lands on when the video is stream-copied.
func (m *Manager) KeyframeBefore(ctx context.Context, input string, t float64) (float64, bool) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	if strings.HasPrefix(input, "http") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1")
	}
	args = append(args, "-ss", strconv.FormatFloat(t, 'f', 3, 64), "-noaccurate_seek", "-copyts", "-start_at_zero",
		"-i", input, "-map", "0:v:0", "-c", "copy", "-frames:v", "1", "-f", "framecrc", "-")
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	probe.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	return parseFramePTS(string(out))
}

func parseFramePTS(out string) (float64, bool) {
	tb, fr := reTimebase.FindStringSubmatch(out), reFrame.FindStringSubmatch(out)
	if tb == nil || fr == nil {
		return 0, false
	}
	num, _ := strconv.ParseFloat(tb[1], 64)
	den, _ := strconv.ParseFloat(tb[2], 64)
	pts, _ := strconv.ParseFloat(fr[1], 64)
	if den == 0 {
		return 0, false
	}
	return max(pts*num/den, 0), true
}

func burnFilter(o Options) string {
	esc := func(p string) string {
		p = filepath.ToSlash(p)
		if runtime.GOOS == "windows" {
			p = strings.Replace(p, ":", `\:`, 1)
		}
		return "'" + strings.ReplaceAll(p, "'", `\'`) + "'"
	}
	if o.BurnSub.External != "" {
		return "subtitles=" + esc(o.BurnSub.External)
	}
	// si counts subtitle streams only, not absolute stream indexes.
	return fmt.Sprintf("subtitles=%s:si=%d", esc(o.Input), o.BurnSubPos)
}

// Get returns a session and marks it as in use.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s != nil {
		s.touch()
	}
	return s
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

func (m *Manager) stopLocked(id string) {
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	delete(m.sessions, id)
	s.cancel()
	go func() {
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
		_ = os.RemoveAll(s.Dir)
	}()
}

// StopFile stops sessions for a file (when the player seeks to a new offset).
func (m *Manager) StopFile(fileID int64, except string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.Opts.FileID == fileID && id != except {
			m.stopLocked(id)
		}
	}
}

type SessionInfo struct {
	ID      string  `json:"id"`
	FileID  int64   `json:"fileId"`
	Speed   float64 `json:"speed"`
	Encoder string  `json:"encoder"`
	Copy    bool    `json:"copy"`
	Age     int     `json:"age"`
}

func (m *Manager) Sessions() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SessionInfo{}
	for _, s := range m.sessions {
		s.mu.Lock()
		out = append(out, SessionInfo{ID: s.ID, FileID: s.Opts.FileID, Speed: s.Speed, Encoder: m.encoder(), Copy: s.Opts.CopyVideo, Age: int(time.Since(s.started).Seconds())})
		s.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Age < out[j].Age })
	return out
}

func (m *Manager) Encoder() string { return m.encoder() }

// Janitor stops idle sessions and trims the subtitle cache.
func (m *Manager) Janitor(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			for id := range m.sessions {
				m.stopLocked(id)
			}
			m.mu.Unlock()
			return
		case <-t.C:
			m.mu.Lock()
			for id, s := range m.sessions {
				if s.idle() > 90*time.Second {
					m.stopLocked(id)
				}
			}
			m.mu.Unlock()
			cleanOld(filepath.Join(m.CacheDir, "subs"), 7*24*time.Hour)
		}
	}
}

func cleanOld(dir string, age time.Duration) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// WaitFile waits for an HLS file to appear (ffmpeg writes with temp_file, so
// a visible file is complete). The playlist additionally needs one segment.
func (s *Session) WaitFile(ctx context.Context, name string) (string, error) {
	p := filepath.Join(s.Dir, filepath.Base(name))
	deadline := time.Now().Add(45 * time.Second)
	for {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			if name != "index.m3u8" {
				return p, nil
			}
			if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(".ts")) {
				return p, nil
			}
		}
		select {
		case <-s.done:
			if st, err := os.Stat(p); err == nil && st.Size() > 0 {
				return p, nil
			}
			if s.err != nil {
				return "", fmt.Errorf("ffmpeg: %v", s.err)
			}
			return "", os.ErrNotExist
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for transcoder")
		}
		s.touch()
	}
}

// ---------- subtitles ----------

// Subtitle returns a WebVTT file for a subtitle track, converting and caching it.
func (m *Manager) Subtitle(ctx context.Context, input string, t store.Track, offset float64) (string, error) {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%s|%.0f", input, t.Index, t.External, offset)))
	dst := filepath.Join(m.CacheDir, "subs", hex.EncodeToString(h[:8])+".vtt")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	var args []string
	if t.External != "" {
		args = []string{"-hide_banner", "-v", "error", "-y"}
		if raw, err := os.ReadFile(t.External); err == nil && !utf8.Valid(raw) {
			args = append(args, "-sub_charenc", "CP1251") // typical for Russian .srt files
		}
		if offset > 0 {
			args = append(args, "-itsoffset", strconv.FormatFloat(-offset, 'f', 3, 64))
		}
		args = append(args, "-i", t.External, "-f", "webvtt", dst)
	} else {
		if isBitmapSub(t.Codec) {
			return "", errors.New("графические субтитры можно только вжечь в видео")
		}
		args = []string{"-hide_banner", "-v", "error", "-y"}
		if offset > 0 {
			args = append(args, "-ss", strconv.FormatFloat(offset, 'f', 3, 64))
		}
		args = append(args, "-i", input, "-map", fmt.Sprintf("0:%d", t.Index), "-f", "webvtt", dst)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	probe.HideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("%v: %s", err, bytes.TrimSpace(out))
	}
	return dst, nil
}

func isBitmapSub(codec string) bool {
	switch codec {
	case "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "pgssub", "dvdsub":
		return true
	}
	return false
}

func IsBitmapSub(codec string) bool { return isBitmapSub(codec) }
