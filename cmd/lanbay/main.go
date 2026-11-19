// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
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
	"golang.org/x/term"
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
		if len(args) != 1 || !term.IsTerminal(int(os.Stdin.Fd())) {
			fatal(errors.New("uso: lanbay set-password requiere una consola interactiva"))
		}
		fmt.Fprint(os.Stderr, "Nueva contraseña: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fatal(fmt.Errorf("leer contraseña: %w", err))
		}
		if err := app.SetPassword(ctx, string(password)); err != nil {
			fatal(err)
		}
		return
	}
	initial, err := app.Initialize(ctx)
	if err != nil {
		fatal(err)
	}
	if initial != "" {
		initialPath := filepath.Join(c.DataDir, "initial-password.txt")
		if err := os.WriteFile(initialPath, []byte(initial+"\n"), 0600); err != nil {
			fatal(fmt.Errorf("guardar contraseña inicial: %w", err))
		}
		if err := config.ProtectFile(initialPath); err != nil {
			fatal(fmt.Errorf("proteger contraseña inicial: %w", err))
		}
		slog.Warn("Contraseña inicial escrita en initial-password.txt protegido; cámbiela tras iniciar sesión")
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
