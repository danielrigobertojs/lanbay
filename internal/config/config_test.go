// SPDX-License-Identifier: Apache-2.0
package config

import (
	"path/filepath"
	"testing"
)

func TestLoadCreatesSecureDefault(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8443" || len(c.AllowedNetworks) == 0 {
		t.Fatalf("unexpected defaults: %#v", c)
	}
	if _, err := Load(filepath.Dir(c.DataDir)); err != nil {
		t.Fatal(err)
	}
}
