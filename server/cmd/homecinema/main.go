// Command homecinema is the HomeCinema media server. The desktop shell (Tauri)
// starts it as a sidecar; it can also run on its own and be opened in a browser.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/thirteenbtw/homecinema/server/internal/api"
	"github.com/thirteenbtw/homecinema/server/internal/config"
	"github.com/thirteenbtw/homecinema/server/internal/events"
	"github.com/thirteenbtw/homecinema/server/internal/library"
	"github.com/thirteenbtw/homecinema/server/internal/probe"
	"github.com/thirteenbtw/homecinema/server/internal/renderer"
	"github.com/thirteenbtw/homecinema/server/internal/store"
	"github.com/thirteenbtw/homecinema/server/internal/tmdb"
	"github.com/thirteenbtw/homecinema/server/internal/torrent"
	"github.com/thirteenbtw/homecinema/server/internal/transcode"
	"github.com/thirteenbtw/homecinema/server/internal/webui"
)

var version = "0.1.0-dev"

func main() {
	cfg := config.Load()
	if cfg.ExitOnStdin {
		// The desktop shell holds our stdin; if it dies, so do we.
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		}()
	}
	setupLog(cfg.DataDir)
	log.Printf("HomeCinema %s, data in %s", version, cfg.DataDir)

	dbPath := filepath.Join(cfg.DataDir, "homecinema.db")
	applyPendingRestore(cfg.DataDir, dbPath)
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bus := events.New()
	prober := &probe.Prober{FFprobe: cfg.FFprobe}
	tm := tmdb.New(filepath.Join(cfg.DataDir, "images"),
		func() string { return st.Setting("tmdbKey", "") },
		func() string { return st.Setting("metaLang", "ru-RU") })
	if st.Setting("metaProvider", "") == "" {
		// Kinopoisk is the default; installs whose library already matched
		// through TMDB keep TMDB so their stored ids stay valid.
		provider := "kinopoisk"
		var n int
		_ = st.DB.QueryRow(`SELECT COUNT(*) FROM media_items WHERE matched IN (1,3)`).Scan(&n)
		if n > 0 && st.Setting("tmdbKey", "") != "" {
			provider = "tmdb"
		}
		_ = st.SetSetting("metaProvider", provider)
	}
	tm.Provider = func() string { return st.Setting("metaProvider", "kinopoisk") }
	tm.KPKey = func() string { return st.Setting("kpKey", "") }
	tm.Mirror = func() tmdb.Mirror {
		return tmdb.Mirror{API: st.Setting("tmdbApi", ""), Image: st.Setting("tmdbImg", "")}
	}
	scanner := library.New(st, prober, tm, bus)

	trans := transcode.New(cfg.FFmpeg, cfg.CacheDir)
	trans.MaxStreams = func() int { n, _ := strconv.Atoi(st.Setting("maxStreams", "2")); return max(n, 1) }
	trans.HWSetting = func() string { return st.Setting("hwaccel", "auto") }
	go trans.HW() // detect encoders in the background
	go trans.Janitor(ctx)

	torrents := torrent.New(st, bus, cfg.DataDir, cfg.CacheDir)
	torrents.Importer = scanner

	cast := renderer.NewManager(st, bus)
	cast.OnProgress = func(profile, item int64, pos, dur float64) {
		if _, err := st.SaveProgress(profile, item, pos, dur, "tv"); err == nil {
			bus.Publish("progress", map[string]any{"profileId": profile, "itemId": item, "position": pos, "duration": dur, "device": "tv"})
		}
	}

	ln, port, err := listen(cfg.Listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := &api.Server{Cfg: cfg, Store: st, Scanner: scanner, TMDB: tm, Trans: trans, Prober: prober, Torrents: torrents,
		Cast: cast, Bus: bus, Web: webui.FS(), Version: version, Port: port}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go scanner.Run(ctx)
	torrents.Start(ctx)
	go cast.Run(ctx)

	// The desktop shell waits for this line to know the port.
	fmt.Printf("HOMECINEMA_READY http://127.0.0.1:%d\n", port)
	log.Printf("listening on %s", ln.Addr())
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()
	<-ctx.Done()
	log.Printf("shutting down")
	shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shut)
	torrents.Close()
}

// listen binds the configured address, moving to the next port if it is taken.
func listen(addr string) (net.Listener, int, error) {
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, err
	}
	port, _ := strconv.Atoi(portS)
	var lastErr error
	for i := 0; i < 10; i++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port+i)))
		if err == nil {
			return ln, port + i, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func setupLog(dataDir string) {
	p := filepath.Join(dataDir, "logs", "homecinema.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags)
}

// applyPendingRestore swaps in a database uploaded via "Restore" (FR-22).
func applyPendingRestore(dataDir, dbPath string) {
	restore := filepath.Join(dataDir, "restore.db")
	if _, err := os.Stat(restore); err != nil {
		return
	}
	bak := dbPath + "." + time.Now().Format("20060102-150405") + ".bak"
	_ = os.Rename(dbPath, bak)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(restore, dbPath); err != nil {
		log.Printf("restore failed: %v", err)
		_ = os.Rename(bak, dbPath)
		return
	}
	log.Printf("database restored from backup (previous saved as %s)", filepath.Base(bak))
}
