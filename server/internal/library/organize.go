package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thirteenbtw/homecinema/server/internal/parse"
)

// Organize moves a finished download into the library layout (FR-T7):
//
//	<movies>/Title (Year)/Title (Year).ext
//	<shows>/Show/Season N/Show - S01E02.ext
//
// External subtitles next to the video are moved along with it.
func Organize(src, moviesRoot, showsRoot string) (string, error) {
	return OrganizeAs(src, src, moviesRoot, showsRoot)
}

// OrganizeAs moves src into the library, naming it after origPath (src may be a
// temporary hard link of a torrent file that must stay in place for seeding).
// Subtitles next to origPath are linked or copied, never moved, in that case.
func OrganizeAs(src, origPath, moviesRoot, showsRoot string) (string, error) {
	r := parse.Parse(origPath)
	ext := strings.ToLower(filepath.Ext(origPath))
	var dst string
	if r.Kind == "episode" {
		show := safeName(r.Title)
		dst = filepath.Join(showsRoot, show, fmt.Sprintf("Season %d", r.Season), fmt.Sprintf("%s - S%02dE%02d%s", show, r.Season, r.Episode, ext))
	} else {
		name := safeName(r.Title)
		if r.Year > 0 {
			name = fmt.Sprintf("%s (%d)", name, r.Year)
		}
		dst = filepath.Join(moviesRoot, name, name+ext)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // already imported
	}
	if err := moveFile(src, dst); err != nil {
		return "", err
	}
	// Subtitles: Movie.srt / Movie.rus.srt
	base := strings.TrimSuffix(origPath, filepath.Ext(origPath))
	dstBase := strings.TrimSuffix(dst, ext)
	if matches, _ := filepath.Glob(globEscape(base) + "*"); matches != nil {
		for _, m := range matches {
			if !parse.SubExts[strings.ToLower(filepath.Ext(m))] {
				continue
			}
			target := dstBase + strings.TrimPrefix(m, base)
			if src == origPath {
				_ = moveFile(m, target)
			} else if os.Link(m, target) != nil {
				_ = copyFile(m, target)
			}
		}
	}
	return dst, nil
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(strings.TrimSpace(s), ".")
	if s == "" {
		s = "Без названия"
	}
	return s
}

func globEscape(s string) string {
	return strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]").Replace(s)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// moveFile renames, or copies and deletes across drives.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst + ".part")
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst + ".part")
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(dst+".part", dst); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}
