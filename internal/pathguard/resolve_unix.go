// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package pathguard

import "path/filepath"

func resolve(path string) (string, error) { return filepath.EvalSymlinks(path) }
