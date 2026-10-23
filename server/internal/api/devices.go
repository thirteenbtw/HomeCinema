package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/thirteenbtw/homecinema/server/internal/netutil"
	"github.com/thirteenbtw/homecinema/server/internal/renderer"
)

type rendererCaps = renderer.Caps

func (s *Server) devices(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.Cast.Devices())
}

func (s *Server) scanDevices(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.Cast.Rescan(r.Context()))
}

func (s *Server) addDevice(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	d, err := s.Cast.AddManual(r.Context(), strings.TrimSpace(b.IP), b.Port)
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, d)
}

func (s *Server) removeDevice(w http.ResponseWriter, r *http.Request) error {
	return s.okOr(w, s.Cast.Remove(r.PathValue("id")))
}

func (s *Server) castStatus(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.Cast.Statuses())
}

// castLoad sends a library file or a torrent stream to a TV (FR-12, FR-14, FR-T5).
func (s *Server) castLoad(w http.ResponseWriter, r *http.Request) error {
	devID := r.PathValue("id")
	var b struct {
		FileID  int64   `json:"fileId"`
		Torrent string  `json:"torrent"`
		File    int     `json:"file"`
		Start   float64 `json:"start"`
		Audio   *int    `json:"audio"`
		Sub     *int    `json:"sub"`
		Quality string  `json:"quality"`
	}
	b.File = -1
	if err := readJSON(r, &b); err != nil {
		return err
	}
	var dev *renderer.DeviceInfo
	for _, d := range s.Cast.Devices() {
		if d.ID == devID {
			d := d
			dev = &d
		}
	}
	if dev == nil {
		return notFound("устройство не найдено")
	}
	q := map[string][]string{"start": {strconv.FormatFloat(b.Start, 'f', 3, 64)}}
	if b.Quality != "" {
		q["quality"] = []string{b.Quality}
	}
	if b.Audio != nil {
		q["audio"] = []string{strconv.Itoa(*b.Audio)}
	}
	if b.Sub != nil && *b.Sub >= 0 {
		// The Cast default receiver gets no side-loaded text tracks from us, so
		// subtitles are burned into the picture.
		q["sub"] = []string{strconv.Itoa(*b.Sub)}
		q["burn"] = []string{"1"}
	}
	host := fmt.Sprintf("http://%s:%d", netutil.LANAddrFor(dev.Addr), s.Port)

	var pb *playback
	var err error
	pid := s.profileID(r)
	itemID := int64(0)
	title := ""
	if b.Torrent != "" {
		pb, err = s.torrentPlayback(r.Context(), b.Torrent, b.File, dev.Caps, q)
		if err != nil {
			return err
		}
		title = pb.Title
	} else {
		f, ferr := s.Store.FileByID(b.FileID)
		if ferr != nil {
			return notFound("файл не найден")
		}
		if _, err := os.Stat(f.Path); err != nil {
			return notFound("файл недоступен")
		}
		pb, err = s.preparePlayback(r.Context(), f, f.Path, dev.Caps, q, "", "")
		if err != nil {
			return err
		}
		itemID = f.ItemID
		if it, _ := s.Store.Item(pid, f.ItemID); it != nil {
			title = it.Title
			if it.Kind == "episode" {
				title = fmt.Sprintf("%s · S%dE%d", it.ShowTitle, it.Season, it.Episode)
			}
		}
	}

	// Rewrite same-origin URLs into token URLs on the LAN address the TV can reach.
	var url, ctype string
	start := b.Start
	if pb.Mode == "hls" {
		url = host + "/api/t/" + s.streamToken("hls:"+pb.Session) + "/hls/" + pb.Session + "/index.m3u8"
		ctype = "application/x-mpegURL"
		start = 0 // the HLS stream itself begins at the requested position
	} else if b.Torrent != "" {
		hash := strings.ToLower(b.Torrent)
		url = host + "/api/t/" + s.streamToken("torrent:"+hash) + strings.TrimPrefix(pb.URL, "/api")
		ctype = "video/mp4"
	} else {
		url = host + "/api/t/" + s.streamToken(fmt.Sprintf("file:%d", b.FileID)) + fmt.Sprintf("/files/%d/direct", b.FileID)
		ctype = "video/mp4"
	}
	if strings.Contains(pb.Summary, "webm") {
		ctype = "video/webm"
	}
	audioLabel, subLabel := "", "выкл."
	audioIdx, subIdx := -1, -1
	for _, a := range pb.Audio {
		if a.Selected {
			audioLabel, audioIdx = a.Label, a.Index
		}
	}
	for _, t := range pb.Subs {
		if t.Selected {
			subLabel, subIdx = t.Label+" (вжечь)", t.Index
		}
	}
	err = s.Cast.Load(r.Context(), devID, renderer.LoadRequest{
		URL: url, ContentType: ctype, Title: title, Start: start, ItemID: itemID, ProfileID: pid, Duration: pb.Duration,
		Offset: pb.Offset, Transcode: pb.Summary, AudioLabel: audioLabel, SubLabel: subLabel,
		FileID: b.FileID, Torrent: b.Torrent, TorrentFile: b.File, Encoder: pb.Encoder, Audio: audioIdx, Sub: subIdx,
	})
	if err != nil {
		return httpError{http.StatusBadGateway, err.Error()}
	}
	return writeJSON(w, map[string]any{"ok": true, "mode": pb.Mode, "summary": pb.Summary, "encoder": pb.Encoder, "audio": pb.Audio, "subs": pb.Subs, "offset": pb.Offset})
}

func (s *Server) castControl(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var b struct {
		Position float64 `json:"position"`
		Level    float64 `json:"level"`
	}
	_ = readJSON(r, &b)
	var err error
	switch r.PathValue("action") {
	case "play":
		err = s.Cast.Play(id)
	case "pause":
		err = s.Cast.Pause(id)
	case "stop":
		s.Cast.Stop(id)
	case "seek":
		err = s.Cast.Seek(id, b.Position)
	case "volume":
		err = s.Cast.Volume(id, b.Level)
	default:
		return notFound("неизвестное действие")
	}
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]bool{"ok": true})
}
