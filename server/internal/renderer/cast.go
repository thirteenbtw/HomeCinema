package renderer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/dns"

	"github.com/thirteenbtw/homecinema/server/internal/store"
)

// DiscoverCast browses mDNS for _googlecast._tcp devices.
func DiscoverCast(ctx context.Context, timeout time.Duration) ([]store.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ch, err := dns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []store.Device
	for e := range ch {
		if e.AddrV4 == nil || seen[e.UUID] {
			continue
		}
		seen[e.UUID] = true
		name := e.DeviceName
		if name == "" {
			name = e.Device
		}
		out = append(out, store.Device{ID: e.UUID, Name: name, Model: e.Device, Proto: "cast", Addr: e.AddrV4.String(), Port: e.Port})
	}
	return out, nil
}

type castRenderer struct {
	dev store.Device
	mu  sync.Mutex
	app *application.Application
	req *LoadRequest
}

func newCast(d store.Device) *castRenderer { return &castRenderer{dev: d} }

func (c *castRenderer) Device() store.Device { return c.dev }
func (c *castRenderer) Caps() Caps           { return CastCaps(c.dev.Model) }

func (c *castRenderer) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *castRenderer) connectLocked() error {
	if c.app != nil {
		return nil
	}
	app := application.NewApplication(application.WithCacheDisabled(true), application.WithConnectionRetries(2))
	done := make(chan error, 1)
	go func() { done <- app.Start(c.dev.Addr, c.dev.Port) }()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-time.After(10 * time.Second):
		return errors.New("нет ответа от устройства")
	}
	c.app = app
	return nil
}

func (c *castRenderer) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app != nil {
		_ = c.app.Close(false)
		c.app = nil
	}
	return c.connectLocked()
}

func (c *castRenderer) friendlyName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app == nil {
		return ""
	}
	if info, err := c.app.Info(); err == nil && info != nil {
		return info.Name
	}
	return ""
}

func (c *castRenderer) Load(ctx context.Context, req LoadRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectLocked(); err != nil {
		return err
	}
	// detach + forceDetach: return once the TV accepted the media.
	if err := c.app.Load(req.URL, int(req.Start), req.ContentType, false, true, true); err != nil {
		return fmt.Errorf("Chromecast: %w", err)
	}
	c.req = &req
	return nil
}

func (c *castRenderer) do(f func(*application.Application) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}
	return f(c.app)
}

func (c *castRenderer) Play() error {
	return c.do(func(a *application.Application) error { return a.Unpause() })
}
func (c *castRenderer) Pause() error {
	return c.do(func(a *application.Application) error { return a.Pause() })
}
func (c *castRenderer) Seek(s float64) error {
	return c.do(func(a *application.Application) error { return a.SeekToTime(float32(s)) })
}
func (c *castRenderer) SetVolume(v float64) error {
	return c.do(func(a *application.Application) error { return a.SetVolume(float32(v)) })
}
func (c *castRenderer) Stop() error {
	return c.do(func(a *application.Application) error { return a.StopMedia() })
}

func (c *castRenderer) Status() (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := Status{DeviceID: c.dev.ID, DeviceName: c.dev.Name, State: "idle"}
	if c.app == nil {
		return st, errors.New("нет соединения")
	}
	if err := c.app.Update(); err != nil {
		return st, err
	}
	_, media, vol := c.app.Status()
	if vol != nil {
		st.Volume, st.Muted = float64(vol.Level), vol.Muted
	}
	if media != nil {
		st.Position = float64(media.CurrentTime)
		st.Duration = float64(media.Media.Duration)
		switch media.PlayerState {
		case "PLAYING":
			st.State = "playing"
		case "PAUSED":
			st.State = "paused"
		case "BUFFERING", "LOADING":
			st.State = "buffering"
		default:
			st.State = "idle"
		}
	}
	return st, nil
}

func (c *castRenderer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app != nil {
		_ = c.app.Close(false)
		c.app = nil
	}
}
