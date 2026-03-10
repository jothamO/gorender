package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Handler serves the optional smooth-player UI assets.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("ui: failed to load embedded static files: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
