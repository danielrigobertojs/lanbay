// SPDX-License-Identifier: Apache-2.0
package server

import (
	"github.com/danielrigobertojs/lanbay/internal/config"
	"net/netip"
	"testing"
)

func TestAllowedIP(t *testing.T) {
	networks := config.DefaultNetworks()
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "fc00::1"} {
		if !AllowedIP(netip.MustParseAddr(s), networks) {
			t.Errorf("%s should be allowed", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "172.15.1.1", "100.64.1.1", "fe00::1"} {
		if AllowedIP(netip.MustParseAddr(s), networks) {
			t.Errorf("%s should be rejected", s)
		}
	}
}
