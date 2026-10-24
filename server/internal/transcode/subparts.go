package transcode

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// subJob extracts an embedded subtitle track from a position to the end of
// the file. Pulling a track out of a big (4K) file or a torrent that is
// still downloading means reading all of it, which takes minutes, so the
// player gets the cues as they are written instead of waiting for the end.
type subJob struct {
	part, dst string
	done      chan struct{}
	err       error
}

var (
	subJobsMu sync.Mutex
	subJobs   = map[string]*subJob{}
)

// SubtitleFrom returns the WebVTT file of track t from position from (in
// movie time) and whether it is complete. While it is not, the file holds
// the cues extracted so far, possibly ending in a half-written one. source
// names the file stably (input may be a URL with a changing token).
func (m *Manager) SubtitleFrom(source, input string, t store.Track, from float64) (path string, done bool, err error) {
	if t.External != "" || isBitmapSub(t.Codec) {
		p, err := m.Subtitle(context.Background(), input, t, 0)
		return p, true, err
	}
	from = max(from, 0)
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|from%.0f", source, t.Index, from)))
	dst := filepath.Join(m.CacheDir, "subs", hex.EncodeToString(h[:8])+".vtt")
	if _, err := os.Stat(dst); err == nil {
		return dst, true, nil
	}
	key := dst
	subJobsMu.Lock()
	j := subJobs[key]
	if j == nil {
		j = &subJob{part: dst + ".part", dst: dst, done: make(chan struct{})}
		subJobs[key] = j
		go m.runSubJob(j, input, t, from)
	}
	subJobsMu.Unlock()
	select {
	case <-j.done:
		subJobsMu.Lock()
		delete(subJobs, key)
		subJobsMu.Unlock()
		if j.err != nil {
			return "", true, j.err
		}
		return j.dst, true, nil
	default:
		return j.part, false, nil
	}
}

func (m *Manager) runSubJob(j *subJob, input string, t store.Track, from float64) {
	defer close(j.done)
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-y", "-nostdin"}
	if from > 0 {
		// -copyts keeps cue times in movie time wherever extraction starts.
		args = append(args, "-ss", strconv.FormatFloat(from, 'f', 3, 64), "-copyts")
	}
	args = append(args, "-i", input, "-map", fmt.Sprintf("0:%d", t.Index), "-flush_packets", "1", "-f", "webvtt", j.part)
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	probe.HideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("subtitles %s #%d: %v: %s", filepath.Base(input), t.Index, err, out)
		j.err = fmt.Errorf("%v: %s", err, out)
		_ = os.Remove(j.part)
		return
	}
	j.err = os.Rename(j.part, j.dst)
}
