// Package web embeds the static files of the UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static is the UI file tree, rooted at static/.
var Static = mustSub(files, "static")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
