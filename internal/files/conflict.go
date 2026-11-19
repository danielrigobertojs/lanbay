// SPDX-License-Identifier: Apache-2.0
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConflictPolicy string

const (
	ConflictOverwrite ConflictPolicy = "overwrite"
	ConflictSkip      ConflictPolicy = "skip"
	ConflictRename    ConflictPolicy = "rename"
)

// ResolveConflict returns the safe destination selected by a user policy.
func ResolveConflict(destination string, policy ConflictPolicy) (string, bool, error) {
	_, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return destination, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect destination: %w", err)
	}
	switch policy {
	case ConflictOverwrite:
		return destination, true, nil
	case ConflictSkip:
		return destination, false, nil
	case ConflictRename:
		dir, base := filepath.Dir(destination), filepath.Base(destination)
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		for i := 1; i < 10000; i++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
			if _, e := os.Lstat(candidate); os.IsNotExist(e) {
				return candidate, true, nil
			} else if e != nil {
				return "", false, fmt.Errorf("inspect conflict candidate: %w", e)
			}
		}
		return "", false, fmt.Errorf("too many name conflicts")
	default:
		return "", false, fmt.Errorf("unknown conflict policy %q", policy)
	}
}
