package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded UI.
// Mount it at /ui in your server mux.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("ui: failed to sub static files: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
