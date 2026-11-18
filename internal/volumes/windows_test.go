// SPDX-License-Identifier: Apache-2.0
package volumes

import "testing"

func TestAcceptedTarget(t *testing.T) {
	for _, tc := range []struct {
		target []string
		want   bool
	}{{[]string{`\Device\HarddiskVolume3`}, true}, {[]string{`\??\C:\folder`}, false}, {[]string{`\Device\LanmanRedirector\x`}, false}, {[]string{`\Device\Mup\x`}, false}, {[]string{`\??\UNC\host\share`}, false}, {[]string{`\Device\HarddiskVolume3`, `extra`}, false}} {
		if got := acceptedTarget(tc.target); got != tc.want {
			t.Errorf("acceptedTarget(%q)=%v", tc.target, got)
		}
	}
}
