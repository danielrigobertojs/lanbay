// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/danielrigobertojs/lanbay/internal/license"
	lanbayweb "github.com/danielrigobertojs/lanbay/internal/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Print(license.Text(version))
		return
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", lanbayweb.Handler())
	server := &http.Server{Addr: ":8080", Handler: mux}
	slog.Info("Lanbay development server listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
