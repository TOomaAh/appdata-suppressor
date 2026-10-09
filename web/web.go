// Package web embeds the built UI (a Svelte app, see package.json).
//
// Run `npm ci && npm run build` in this directory before `go build`; without it
// the binary still builds (dist/.gitkeep keeps the embed valid) but serves no UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Static is the built UI file tree, rooted at dist/.
var Static = mustSub(files, "dist")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
