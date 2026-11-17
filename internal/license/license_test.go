// SPDX-License-Identifier: Apache-2.0
package license

import (
	"strings"
	"testing"
)

func TestTextIncludesVersionAndAttribution(t *testing.T) {
	text := Text("v0.0.0")
	for _, want := range []string{"v0.0.0", "Daniel Rigoberto Jacobo Sandoval", "Apache-2.0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Text() missing %q: %q", want, text)
		}
	}
}
