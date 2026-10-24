package api

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/transcode"
)

func TestDecideBrowser4K(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = st.SetSetting("tonemap", "0")
	s := &Server{Store: st, Trans: transcode.New("ffmpeg-missing", t.TempDir())}
	caps := browserCaps(httptest.NewRequest("GET", "/?hevc=1&mkv=1", nil))
	uhd := func(hdr, audio string) (*store.File, *store.Track) {
		return &store.File{VideoCodec: "hevc", BitDepth: 10, Width: 3840, Height: 2160, HDR: hdr, Container: "mkv"}, &store.Track{Codec: audio}
	}
	cases := []struct {
		name         string
		hdr, audio   string
		q            string
		direct, copy bool
	}{
		{"HDR10 HEVC with AAC plays as is on an SDR screen", "HDR10", "aac", "", true, false},
		{"DV with an HDR10 base plays as is", "DV HDR10", "aac", "", true, false},
		{"AC3 audio: video is copied, only sound is converted", "HDR10", "ac3", "", false, true},
		{"DV profile 5 is transcoded", "DV", "aac", "", false, false},
		{"after a failed direct open the file is remuxed", "", "aac", "nodirect", false, true},
	}
	for _, c := range cases {
		cp := caps
		cp.NoDirect = c.q == "nodirect"
		f, a := uhd(c.hdr, c.audio)
		d := s.decide(f, a, cp, "original", true)
		if d.Direct != c.direct || !d.Direct && d.CopyVideo != c.copy {
			t.Errorf("%s: got direct=%v copy=%v (%s; %s)", c.name, d.Direct, d.CopyVideo, d.Summary, d.Reason)
		}
	}
	// Without the Matroska flag MKV is remuxed, never re-encoded.
	f, a := uhd("HDR10", "aac")
	plain := browserCaps(httptest.NewRequest("GET", "/?hevc=1", nil))
	if d := s.decide(f, a, plain, "original", true); d.Direct || !d.CopyVideo || !d.CopyAudio {
		t.Errorf("mkv without mkv=1: %+v", d)
	}
}
