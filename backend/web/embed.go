package web

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assets embed.FS

func Assets() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
