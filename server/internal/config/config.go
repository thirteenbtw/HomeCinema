// Package config resolves where HomeCinema keeps its data and how the server starts.
package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	DataDir   string // SQLite DB, metadata cache, logs
	CacheDir  string // transcode segments, torrent stream cache
	Listen    string // HTTP listen address
	FFmpeg    string // path to ffmpeg binary
	FFprobe   string // path to ffprobe binary
	Dev       bool   // allow CORS from the Vite dev server
	OpenUIURL bool
}

func Load() Config {
	var c Config
	flag.StringVar(&c.DataDir, "data", defaultDataDir(), "data directory")
	flag.StringVar(&c.Listen, "listen", "0.0.0.0:8096", "HTTP listen address (only LAN clients are accepted)")
	flag.StringVar(&c.FFmpeg, "ffmpeg", "", "path to ffmpeg (default: next to the binary, then PATH)")
	flag.StringVar(&c.FFprobe, "ffprobe", "", "path to ffprobe (default: next to the binary, then PATH)")
	flag.BoolVar(&c.Dev, "dev", false, "development mode (CORS for the Vite dev server)")
	flag.Parse()

	c.CacheDir = filepath.Join(c.DataDir, "cache")
	if c.FFmpeg == "" {
		c.FFmpeg = findTool("ffmpeg")
	}
	if c.FFprobe == "" {
		c.FFprobe = findTool("ffprobe")
	}
	for _, d := range []string{c.DataDir, c.CacheDir, filepath.Join(c.DataDir, "images"), filepath.Join(c.DataDir, "logs")} {
		_ = os.MkdirAll(d, 0o755)
	}
	return c
}

func defaultDataDir() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "HomeCinema")
		}
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "HomeCinema")
	}
	return ".homecinema"
}

// findTool prefers a bundled binary next to the executable (the installer ships
// ffmpeg there), then falls back to PATH lookup by bare name.
func findTool(name string) string {
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		for _, p := range []string{filepath.Join(dir, exe), filepath.Join(dir, "ffmpeg", exe)} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return name
}
