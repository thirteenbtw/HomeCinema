package transcode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// Cues extracted from a position keep movie time and the job finishes.
func TestSubtitleFrom(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "s.srt")
	var b strings.Builder
	for i := 0; i < 40; i++ {
		s := i * 5
		fmt.Fprintf(&b, "%d\n00:%02d:%02d,000 --> 00:%02d:%02d,000\nline %d\n\n", i+1, s/60, s%60, (s+3)/60, (s+3)%60, i)
	}
	_ = os.WriteFile(srt, []byte(b.String()), 0o644)
	mkv := filepath.Join(dir, "m.mkv")
	if out, err := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=64x64:rate=10", "-i", srt, "-t", "200",
		"-map", "0", "-map", "1", "-c:v", "libx264", "-preset", "ultrafast", "-g", "20", "-c:s", "srt", mkv).CombinedOutput(); err != nil {
		t.Skipf("cannot make a test file: %v %s", err, out)
	}
	m := New(ff, dir)
	tr := store.Track{Index: 1, Codec: "subrip"}
	var p string
	var done bool
	for deadline := time.Now().Add(30 * time.Second); !done; time.Sleep(100 * time.Millisecond) {
		if p, done, err = m.SubtitleFrom("m", mkv, tr, 100); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("not done")
		}
	}
	raw, _ := os.ReadFile(p)
	got := string(raw)
	if !strings.Contains(got, "01:40.000 --> 01:43.000\nline 20") || strings.Contains(got, "line 19\n") {
		t.Fatalf("unexpected cues:\n%s", got)
	}
}
