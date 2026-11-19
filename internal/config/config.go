// SPDX-License-Identifier: Apache-2.0
// Package config loads the small, deliberately explicit Lanbay TOML configuration.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const fileName = "config.toml"

type Config struct {
	DataDir          string
	Listen           string
	CertificateFile  string
	KeyFile          string
	AllowedNetworks  []netip.Prefix
	SessionIdle      time.Duration
	DiskAlertPercent int
}

func Default(dataDir string) Config {
	return Config{DataDir: dataDir, Listen: ":8443", CertificateFile: filepath.Join(dataDir, "tls-cert.pem"), KeyFile: filepath.Join(dataDir, "tls-key.pem"), AllowedNetworks: DefaultNetworks(), SessionIdle: 12 * time.Hour, DiskAlertPercent: 90}
}

func DefaultNetworks() []netip.Prefix {
	return []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("fc00::/7")}
}

func Load(dataDir string) (Config, error) {
	c := Default(dataDir)
	path := filepath.Join(dataDir, fileName)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := ensureDataDir(dataDir); err != nil {
			return c, fmt.Errorf("create data directory: %w", err)
		}
		return c, Save(c)
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := ensureDataDir(dataDir); err != nil {
		return c, fmt.Errorf("verify data directory: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), "\"")
		switch key {
		case "listen":
			c.Listen = value
		case "certificate_file":
			c.CertificateFile = value
		case "key_file":
			c.KeyFile = value
		case "session_idle_hours":
			h, e := strconv.Atoi(value)
			if e != nil || h < 1 {
				return c, fmt.Errorf("invalid session_idle_hours")
			}
			c.SessionIdle = time.Duration(h) * time.Hour
		case "disk_alert_percent":
			p, e := strconv.Atoi(value)
			if e != nil || p < 1 || p > 100 {
				return c, fmt.Errorf("invalid disk_alert_percent")
			}
			c.DiskAlertPercent = p
		case "allowed_networks":
			prefixes, e := parsePrefixes(value)
			if e != nil {
				return c, e
			}
			c.AllowedNetworks = prefixes
		}
	}
	return c, nil
}

func Save(c Config) error {
	if err := ensureDataDir(c.DataDir); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	prefixes := make([]string, 0, len(c.AllowedNetworks))
	for _, p := range c.AllowedNetworks {
		prefixes = append(prefixes, "\""+p.String()+"\"")
	}
	content := fmt.Sprintf("listen = %q\ncertificate_file = %q\nkey_file = %q\nsession_idle_hours = %d\ndisk_alert_percent = %d\nallowed_networks = [%s]\n", c.Listen, c.CertificateFile, c.KeyFile, int(c.SessionIdle.Hours()), c.DiskAlertPercent, strings.Join(prefixes, ", "))
	return os.WriteFile(filepath.Join(c.DataDir, fileName), []byte(content), 0600)
}

func parsePrefixes(value string) ([]netip.Prefix, error) {
	value = strings.Trim(value, "[] ")
	if value == "" {
		return nil, fmt.Errorf("allowed_networks cannot be empty")
	}
	var result []netip.Prefix
	for _, item := range strings.Split(value, ",") {
		p, err := netip.ParsePrefix(strings.Trim(strings.TrimSpace(item), "\""))
		if err != nil {
			return nil, fmt.Errorf("invalid allowed network: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}
