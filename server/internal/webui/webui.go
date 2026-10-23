// Package webui embeds the built React app (web/ → npm run build → dist here).
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func FS() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
