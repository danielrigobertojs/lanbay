// SPDX-License-Identifier: Apache-2.0
// Package config loads the small, deliberately explicit Lanbay TOML configuration.
package config

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const fileName = "config.toml"

const hostCacheTTL = 30 * time.Second

var hostCache struct {
	sync.Mutex
	hosts   []string
	expires time.Time
}

type Config struct {
	DataDir          string
	Listen           string
	CertificateFile  string
	KeyFile          string
	AllowedNetworks  []netip.Prefix
	SessionIdle      time.Duration
	DiskAlertPercent int
	AllowedHosts     []string
	ProtectedPaths   []string
	MaxUploadBytes   int64
}

func Default(dataDir string) Config {
	return Config{DataDir: dataDir, Listen: ":8443", CertificateFile: filepath.Join(dataDir, "tls-cert.pem"), KeyFile: filepath.Join(dataDir, "tls-key.pem"), AllowedNetworks: DefaultNetworks(), AllowedHosts: DefaultHosts(), SessionIdle: 12 * time.Hour, DiskAlertPercent: 90, MaxUploadBytes: 128 << 20}
}
func DefaultHosts() []string {
	h, _ := os.Hostname()
	return []string{"localhost", h, h + ".local"}
}

// RuntimeHosts returns the local interface addresses for Host validation.
// They are intentionally not persisted: addresses can change between boots.
func RuntimeHosts() []string {
	now := time.Now()
	hostCache.Lock()
	defer hostCache.Unlock()
	if now.Before(hostCache.expires) {
		return append([]string(nil), hostCache.hosts...)
	}
	hostCache.hosts = append(DefaultHosts(), "127.0.0.1", "::1")
	hostCache.hosts = append(hostCache.hosts, interfaceIPs()...)
	hostCache.expires = now.Add(hostCacheTTL)
	return append([]string(nil), hostCache.hosts...)
}

func interfaceIPs() []string {
	addrs, _ := net.InterfaceAddrs()
	var result []string
	for _, a := range addrs {
		if ip, _, err := net.ParseCIDR(a.String()); err == nil && !ip.IsLoopback() {
			result = append(result, ip.String())
		}
	}
	return result
}

func DefaultNetworks() []netip.Prefix {
	return []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("fc00::/7")}
}

func Load(dataDir string) (Config, error) {
	c := Default(dataDir)
	if err := ensureDataDir(dataDir); err != nil {
		return c, fmt.Errorf("secure data directory: %w", err)
	}
	path := filepath.Join(dataDir, fileName)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, Save(c)
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
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
		case "allowed_hosts":
			c.AllowedHosts = splitStrings(value)
		case "protected_paths":
			c.ProtectedPaths = splitStrings(value)
		case "max_upload_bytes":
			n, e := strconv.ParseInt(value, 10, 64)
			if e != nil || n < 1 {
				return c, fmt.Errorf("invalid max_upload_bytes")
			}
			c.MaxUploadBytes = n
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
	hosts := make([]string, 0, len(c.AllowedHosts))
	for _, host := range c.AllowedHosts {
		hosts = append(hosts, "\""+host+"\"")
	}
	protected := make([]string, 0, len(c.ProtectedPaths))
	for _, path := range c.ProtectedPaths {
		protected = append(protected, "\""+path+"\"")
	}
	content := fmt.Sprintf("listen = %q\ncertificate_file = %q\nkey_file = %q\nsession_idle_hours = %d\ndisk_alert_percent = %d\nallowed_networks = [%s]\nallowed_hosts = [%s]\nprotected_paths = [%s]\nmax_upload_bytes = %d\n", c.Listen, c.CertificateFile, c.KeyFile, int(c.SessionIdle.Hours()), c.DiskAlertPercent, strings.Join(prefixes, ", "), strings.Join(hosts, ", "), strings.Join(protected, ", "), c.MaxUploadBytes)
	return os.WriteFile(filepath.Join(c.DataDir, fileName), []byte(content), 0600)
}
func splitStrings(value string) []string {
	value = strings.Trim(value, "[] ")
	if value == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		result = append(result, strings.Trim(strings.TrimSpace(item), "\""))
	}
	return result
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
