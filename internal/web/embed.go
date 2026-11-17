// SPDX-License-Identifier: Apache-2.0
// Package web serves the compiled local web interface.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var assets embed.FS

// Handler returns a handler for the bundled single-page interface.
func Handler() http.Handler {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(dist))
}
