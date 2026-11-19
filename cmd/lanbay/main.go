// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	if _, err := server.EnsureCertificate(c.CertificateFile, c.KeyFile); err != nil {
		fatal(err)
	}
	if fingerprint, err := server.CertificateFingerprint(c.CertificateFile); err == nil {
		slog.Info("Huella del certificado TLS", "fingerprint", fingerprint)
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
	httpServer := &http.Server{Addr: c.Listen, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
	slog.Info("Lanbay HTTPS listening", "address", c.Listen)
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		fatal(err)
	}
	if err := httpServer.ServeTLS(limitListener(listener, 64), c.CertificateFile, c.KeyFile); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

type limitedListener struct {
	net.Listener
	slots chan struct{}
}

func limitListener(listener net.Listener, max int) net.Listener {
	return &limitedListener{Listener: listener, slots: make(chan struct{}, max)}
}
func (l *limitedListener) Accept() (net.Conn, error) {
	l.slots <- struct{}{}
	connection, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedConn{Conn: connection, once: sync.Once{}, release: func() { <-l.slots }}, nil
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }

func fatal(err error) { slog.Error("Lanbay stopped", "error", err); os.Exit(1) }
