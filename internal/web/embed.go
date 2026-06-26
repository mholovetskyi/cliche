package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticFiles embed.FS

// StaticFS returns the embedded Studio UI assets, rooted at the static dir, so
// the single binary serves the whole front-end with no external files.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return staticFiles
	}
	return sub
}
