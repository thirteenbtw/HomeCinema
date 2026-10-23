// Package renderer sends playback to TVs. v1 implements Google Cast
// (Chromecast, Android TV); DLNA and AirPlay plug into the same Renderer
// interface later (FR-11).
package renderer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// Caps describes what a device decodes natively (FR-14).
type Caps struct {
	Video     []string `json:"video"` // h264, hevc, vp9, av1
	Audio     []string `json:"audio"` // aac, mp3, opus, ac3, eac3, flac
	Container []string `json:"container"`
	MaxHeight int      `json:"maxHeight"`
	HDR       bool     `json:"hdr"`
	TextSubs  bool     `json:"textSubs"`
}

func (c Caps) Has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

type LoadRequest struct {
	URL         string
	ContentType string
	Title       string
	Start       float64
	ItemID      int64
	ProfileID   int64
	Duration    float64
	Offset      float64 // an HLS transcode that starts mid-movie: TV time 0 = Offset
	Transcode   string  // human-readable transcode summary for the remote
	AudioLabel  string
	SubLabel    string
}

type Status struct {
	DeviceID   string  `json:"deviceId"`
	DeviceName string  `json:"deviceName"`
	State      string  `json:"state"` // idle | buffering | playing | paused | error | connecting
	Position   float64 `json:"position"`
	Duration   float64 `json:"duration"`
	Volume     float64 `json:"volume"`
	Muted      bool    `json:"muted"`
	ItemID     int64   `json:"itemId"`
	Title      string  `json:"title"`
	Transcode  string  `json:"transcode"`
	AudioLabel string  `json:"audioLabel"`
	SubLabel   string  `json:"subLabel"`
	Error      string  `json:"error,omitempty"`
}

// Renderer is one TV/receiver: discover, load, play, pause, seek, stop, status.
type Renderer interface {
	Device() store.Device
	Caps() Caps
	Load(ctx context.Context, req LoadRequest) error
	Play() error
	Pause() error
	Seek(seconds float64) error
	SetVolume(level float64) error
	Stop() error
	Status() (Status, error)
	Close()
}

type DeviceInfo struct {
	store.Device
	Online bool   `json:"online"`
	Busy   bool   `json:"busy"`
	Caps   Caps   `json:"caps"`
	State  string `json:"state"`
}

// ProgressFunc is called while something plays on a TV, to keep the position
// in sync with the PC (FR-15).
type ProgressFunc func(profile, item int64, position, duration float64)

type Manager struct {
	Store      *store.Store
	Bus        *events.Bus
	OnProgress ProgressFunc

	mu       sync.Mutex
	seen     map[string]time.Time
	active   map[string]Renderer
	sessions map[string]*LoadRequest
	loadedAt map[string]time.Time
	lastErr  map[string]string
}

func NewManager(s *store.Store, bus *events.Bus) *Manager {
	return &Manager{Store: s, Bus: bus, seen: map[string]time.Time{}, active: map[string]Renderer{}, sessions: map[string]*LoadRequest{}, loadedAt: map[string]time.Time{}, lastErr: map[string]string{}}
}

func (m *Manager) Run(ctx context.Context) {
	go m.discoverLoop(ctx)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			for _, r := range m.active {
				r.Close()
			}
			m.mu.Unlock()
			return
		case <-t.C:
		}
		n++
		m.pollActive(n%5 == 0)
	}
}

func (m *Manager) discoverLoop(ctx context.Context) {
	for {
		found, err := DiscoverCast(ctx, 4*time.Second)
		if err != nil {
			log.Printf("cast discovery: %v", err)
		}
		m.mu.Lock()
		for _, d := range found {
			m.seen[d.ID] = time.Now()
			_ = m.Store.SaveDevice(d)
		}
		m.mu.Unlock()
		m.Bus.Publish("devices", m.Devices())
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

// Rescan triggers an immediate discovery round.
func (m *Manager) Rescan(ctx context.Context) []DeviceInfo {
	found, _ := DiscoverCast(ctx, 4*time.Second)
	m.mu.Lock()
	for _, d := range found {
		m.seen[d.ID] = time.Now()
		_ = m.Store.SaveDevice(d)
	}
	m.mu.Unlock()
	list := m.Devices()
	m.Bus.Publish("devices", list)
	return list
}

func (m *Manager) Devices() []DeviceInfo {
	devs, _ := m.Store.Devices()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []DeviceInfo{}
	for _, d := range devs {
		info := DeviceInfo{Device: d, Caps: CastCaps(d.Model)}
		info.Online = d.Manual || time.Since(m.seen[d.ID]) < 2*time.Minute
		if _, ok := m.active[d.ID]; ok {
			info.Busy = true
			info.Online = true
		}
		out = append(out, info)
	}
	return out
}

// AddManual registers a device by IP (FR-12 fallback) after probing it.
func (m *Manager) AddManual(ctx context.Context, ip string, port int) (store.Device, error) {
	if port == 0 {
		port = 8009
	}
	if net.ParseIP(ip) == nil {
		return store.Device{}, errors.New("неверный IP-адрес")
	}
	d := store.Device{ID: "manual-" + ip, Name: "ТВ " + ip, Proto: "cast", Addr: ip, Port: port, Manual: true}
	c := newCast(d)
	defer c.Close()
	if err := c.connect(); err != nil {
		return d, fmt.Errorf("устройство не отвечает по Google Cast: %w", err)
	}
	if name := c.friendlyName(); name != "" {
		d.Name = name
	}
	if err := m.Store.SaveDevice(d); err != nil {
		return d, err
	}
	m.Bus.Publish("devices", m.Devices())
	return d, nil
}

func (m *Manager) Remove(id string) error {
	m.Stop(id)
	return m.Store.DeleteDevice(id)
}

func (m *Manager) device(id string) (store.Device, error) {
	devs, _ := m.Store.Devices()
	for _, d := range devs {
		if d.ID == id {
			return d, nil
		}
	}
	return store.Device{}, errors.New("устройство не найдено")
}

func (m *Manager) Caps(id string) Caps {
	d, err := m.device(id)
	if err != nil {
		return CastCaps("")
	}
	return CastCaps(d.Model)
}

// Load starts playback on a device, reconnecting a few times if needed.
func (m *Manager) Load(ctx context.Context, id string, req LoadRequest) error {
	d, err := m.device(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	r, ok := m.active[id]
	m.mu.Unlock()
	if !ok {
		r = newCast(d)
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if lastErr = r.Load(ctx, req); lastErr == nil {
			break
		}
		r.Close()
		r = newCast(d)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	if lastErr != nil {
		return lastErr
	}
	m.mu.Lock()
	m.active[id] = r
	m.sessions[id] = &req
	m.loadedAt[id] = time.Now()
	delete(m.lastErr, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) with(id string, f func(Renderer) error) error {
	m.mu.Lock()
	r, ok := m.active[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("на этом устройстве ничего не воспроизводится")
	}
	return f(r)
}

func (m *Manager) Play(id string) error {
	return m.with(id, func(r Renderer) error { return r.Play() })
}
func (m *Manager) Pause(id string) error {
	return m.with(id, func(r Renderer) error { return r.Pause() })
}
func (m *Manager) Seek(id string, s float64) error {
	m.mu.Lock()
	off := 0.0
	if req := m.sessions[id]; req != nil {
		off = req.Offset
	}
	m.mu.Unlock()
	return m.with(id, func(r Renderer) error { return r.Seek(max(s-off, 0)) })
}
func (m *Manager) Volume(id string, v float64) error {
	return m.with(id, func(r Renderer) error { return r.SetVolume(v) })
}

// Session returns what is loaded on a device (used to switch tracks: reload
// with the same item at the current position).
func (m *Manager) Session(id string) (*LoadRequest, Status) {
	m.mu.Lock()
	r, ok := m.active[id]
	req := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, Status{}
	}
	st, _ := r.Status()
	return req, st
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	r, ok := m.active[id]
	req := m.sessions[id]
	delete(m.active, id)
	delete(m.sessions, id)
	m.mu.Unlock()
	if !ok {
		return
	}
	if st, err := r.Status(); err == nil && req != nil && m.OnProgress != nil && st.Position > 0 && req.ItemID > 0 {
		m.OnProgress(req.ProfileID, req.ItemID, st.Position+req.Offset, req.Duration)
	}
	_ = r.Stop()
	r.Close()
	m.publishStatus()
}

func (m *Manager) pollActive(saveProgress bool) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.mu.Lock()
		r := m.active[id]
		req := m.sessions[id]
		m.mu.Unlock()
		if r == nil {
			continue
		}
		st, err := r.Status()
		if err != nil {
			// TV went away (Wi-Fi drop): keep the session, try to reconnect quietly.
			m.mu.Lock()
			m.lastErr[id] = err.Error()
			m.mu.Unlock()
			if c, ok := r.(*castRenderer); ok {
				_ = c.reconnect()
			}
			continue
		}
		if req != nil && req.ItemID > 0 && saveProgress && m.OnProgress != nil && st.Position > 0 && (st.State == "playing" || st.State == "paused") {
			m.OnProgress(req.ProfileID, req.ItemID, st.Position+req.Offset, req.Duration)
		}
		m.mu.Lock()
		settled := time.Since(m.loadedAt[id]) > 20*time.Second
		m.mu.Unlock()
		if st.State == "idle" && settled {
			// Finished, or stopped with the TV's own remote.
			m.mu.Lock()
			delete(m.active, id)
			delete(m.sessions, id)
			m.mu.Unlock()
			r.Close()
		}
	}
	m.publishStatus()
}

func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Status{}
	for id, r := range m.active {
		st, err := r.Status()
		req := m.sessions[id]
		if err != nil {
			st = Status{DeviceID: id, State: "connecting", Error: m.lastErr[id]}
		}
		if req != nil {
			st.Position += req.Offset
			st.ItemID, st.Title, st.Transcode, st.AudioLabel, st.SubLabel = req.ItemID, req.Title, req.Transcode, req.AudioLabel, req.SubLabel
			if req.Duration > 0 {
				st.Duration = req.Duration // HLS event playlists report a growing duration
			}
		}
		st.DeviceID = id
		st.DeviceName = r.Device().Name
		out = append(out, st)
	}
	return out
}

func (m *Manager) publishStatus() { m.Bus.Publish("cast", m.Statuses()) }

// CastCaps estimates decoder support from the model name reported over mDNS.
func CastCaps(model string) Caps {
	md := strings.ToLower(model)
	c := Caps{Video: []string{"h264"}, Audio: []string{"aac", "mp3", "opus", "vorbis", "flac"}, Container: []string{"mp4", "webm"}, MaxHeight: 1080, TextSubs: true}
	switch {
	case strings.Contains(md, "google tv") || strings.Contains(md, "android") || strings.Contains(md, "shield") || strings.Contains(md, "bravia") || strings.Contains(md, "tv"):
		c.Video = append(c.Video, "hevc", "vp9", "av1")
		c.Audio = append(c.Audio, "ac3", "eac3")
		c.Container = append(c.Container, "mkv")
		c.MaxHeight, c.HDR = 2160, true
	case strings.Contains(md, "ultra"):
		c.Video = append(c.Video, "hevc", "vp9")
		c.MaxHeight, c.HDR = 2160, true
	}
	return c
}
