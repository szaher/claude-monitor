package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var content embed.FS

// StaticFiles returns a filesystem rooted at the static/ directory,
// suitable for use with http.FileServer.
func StaticFiles() fs.FS {
	sub, _ := fs.Sub(content, "static")
	return sub
}
