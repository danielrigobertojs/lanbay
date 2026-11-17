// SPDX-License-Identifier: Apache-2.0
// Package license exposes required distribution attribution.
package license

import "fmt"

const copyright = "Copyright 2026 Daniel Rigoberto Jacobo Sandoval"

// Text returns the version and the attribution shipped with the binary.
func Text(version string) string {
	return fmt.Sprintf("lanbay %s\n%s\nLicensed under Apache-2.0. See NOTICE and THIRD_PARTY_LICENSES.md.\n", version, copyright)
}
