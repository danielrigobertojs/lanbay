// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/license"
	"github.com/danielrigobertojs/lanbay/internal/server"
	"github.com/danielrigobertojs/lanbay/internal/store"
	"github.com/danielrigobertojs/lanbay/internal/volumes"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "version" {
		fmt.Print(license.Text(version))
		return
	}
	ctx := context.Background()
	c, err := config.Load(config.DefaultDataDir())
	if err != nil {
		fatal(err)
	}
	db, err := store.Open(ctx, filepath.Join(c.DataDir, "lanbay.sqlite"))
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	app := server.New(c, db, volumes.NewProvider(), version)
	if len(args) > 0 && args[0] == "set-password" {
		if len(args) != 2 {
			fatal(fmt.Errorf("uso: lanbay set-password <contraseña>"))
		}
		if err := app.SetPassword(ctx, args[1]); err != nil {
			fatal(err)
		}
		return
	}
	initial, err := app.Initialize(ctx)
	if err != nil {
		fatal(err)
	}
	if initial != "" {
		slog.Warn("Contraseña inicial generada; cámbiela tras iniciar sesión", "password", initial)
	}
	if _, err := server.EnsureCertificate(c.CertificateFile, c.KeyFile); err != nil {
		fatal(err)
	}
	httpServer := &http.Server{Addr: c.Listen, Handler: app.Handler(), ReadHeaderTimeout: 10e9}
	slog.Info("Lanbay HTTPS listening", "address", c.Listen)
	if err := httpServer.ListenAndServeTLS(c.CertificateFile, c.KeyFile); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func fatal(err error) { slog.Error("Lanbay stopped", "error", err); os.Exit(1) }
