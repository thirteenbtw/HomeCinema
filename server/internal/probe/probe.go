// Package probe reads codecs, tracks and duration of a media file with ffprobe.
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/parse"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

type Prober struct {
	FFprobe string
}

type ffStream struct {
	Index         int               `json:"index"`
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	Profile       string            `json:"profile"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	PixFmt        string            `json:"pix_fmt"`
	Channels      int               `json:"channels"`
	ColorTransfer string            `json:"color_transfer"`
	BitsPerRaw    string            `json:"bits_per_raw_sample"`
	Tags          map[string]string `json:"tags"`
	Disposition   map[string]int    `json:"disposition"`
	SideData      []map[string]any  `json:"side_data_list"`
}

type ffOut struct {
	Streams []ffStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

// Probe fills a store.File for path. External subtitles next to the video
// (movie.srt, movie.ru.srt, Subs/movie.ass) are added as extra tracks.
func (p *Prober) Probe(ctx context.Context, path string) (store.File, error) {
	f := store.File{Path: path, BitDepth: 8}
	st, err := os.Stat(path)
	if err != nil {
		return f, err
	}
	f.Size = st.Size()
	f.Mtime = st.ModTime().Unix()

	err = p.run(ctx, path, &f, time.Minute)
	f.Subs = append(f.Subs, External(path)...)
	return f, err
}

// ProbeURL probes a stream served over HTTP (a torrent file being
// downloaded). It may have to wait for the pieces holding the start and the
// end of the file, hence the long timeout.
func (p *Prober) ProbeURL(ctx context.Context, url string) (store.File, error) {
	f := store.File{Path: url, BitDepth: 8}
	err := p.run(ctx, url, &f, 3*time.Minute)
	return f, err
}

func (p *Prober) run(ctx context.Context, path string, f *store.File, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		// A killed process reports "exit status 1" on Windows: name the timeout.
		if ctx.Err() != nil {
			return fmt.Errorf("не уложились в %d мин", int(timeout.Minutes()))
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(bytes.TrimSpace(ee.Stderr)) > 0 {
			return fmt.Errorf("%v: %s", err, lastLine(ee.Stderr))
		}
		return err
	}
	var r ffOut
	if err := json.Unmarshal(out, &r); err != nil {
		return err
	}
	f.Container = containerName(r.Format.FormatName, path)
	f.Duration, _ = strconv.ParseFloat(r.Format.Duration, 64)
	videoSeen := false
	for _, s := range r.Streams {
		lang := s.Tags["language"]
		title := s.Tags["title"]
		switch s.CodecType {
		case "video":
			if videoSeen || s.Disposition["attached_pic"] == 1 {
				continue
			}
			videoSeen = true
			f.VideoCodec = s.CodecName
			f.Width, f.Height = s.Width, s.Height
			if strings.Contains(s.PixFmt, "10") || s.BitsPerRaw == "10" || strings.Contains(s.Profile, "10") {
				f.BitDepth = 10
			}
			f.HDR = hdrKind(s)
		case "audio":
			f.Audio = append(f.Audio, store.Track{Index: s.Index, Codec: s.CodecName, Lang: lang, Title: title, Channels: s.Channels, Default: s.Disposition["default"] == 1})
		case "subtitle":
			f.Subs = append(f.Subs, store.Track{Index: s.Index, Codec: s.CodecName, Lang: lang, Title: title, Default: s.Disposition["default"] == 1, Forced: s.Disposition["forced"] == 1})
		}
	}
	return nil
}

func hdrKind(s ffStream) string {
	for _, sd := range s.SideData {
		if t, _ := sd["side_data_type"].(string); strings.Contains(strings.ToLower(t), "dovi") {
			// Profiles 8.1/8.4 carry an HDR10/HLG base layer any HDR player
			// shows correctly; profile 5 (compatibility id 0) does not.
			switch compat, _ := sd["dv_bl_signal_compatibility_id"].(float64); compat {
			case 1, 6:
				return "DV HDR10"
			case 4:
				return "DV HLG"
			}
			return "DV"
		}
	}
	switch s.ColorTransfer {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	return ""
}

func containerName(format, path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch {
	case strings.Contains(format, "matroska") && ext == "webm":
		return "webm"
	case strings.Contains(format, "matroska"):
		return "mkv"
	case strings.Contains(format, "mp4"):
		return "mp4"
	case strings.Contains(format, "avi"):
		return "avi"
	case strings.Contains(format, "mpegts"):
		return "ts"
	}
	return ext
}

// External finds subtitle files that belong to the video at path.
func External(path string) []store.Track {
	dir := filepath.Dir(path)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	var out []store.Track
	scan := func(d string, loose bool) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if !parse.SubExts[ext] || ext == ".sub" {
				continue
			}
			stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
			if !loose && stem != base && !strings.HasPrefix(stem, base+".") {
				continue
			}
			if loose && !strings.HasPrefix(stem, base) && len(entries) > 30 {
				continue
			}
			lang, title := guessLang(strings.TrimPrefix(stem, base))
			out = append(out, store.Track{Index: -1, Codec: strings.TrimPrefix(ext, "."), Lang: lang, Title: title, External: filepath.Join(d, name)})
		}
	}
	scan(dir, false)
	for _, sub := range []string{"Subs", "subs", "Subtitles", "Субтитры", "RUS Subs"} {
		scan(filepath.Join(dir, sub), true)
	}
	return out
}

func guessLang(suffix string) (string, string) {
	s := strings.Trim(strings.ToLower(suffix), ". _-")
	switch {
	case s == "":
		return "", ""
	case strings.Contains(s, "rus") || s == "ru" || strings.Contains(s, "рус"):
		return "rus", ""
	case strings.Contains(s, "eng") || s == "en":
		return "eng", ""
	}
	return "", strings.Trim(suffix, ". _-")
}

func lastLine(b []byte) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return string(bytes.TrimSpace(lines[len(lines)-1]))
}
