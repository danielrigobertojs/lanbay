// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package pathguard

import "os"

func resolveOpen(path string, _ bool) (*Resolved, error) {
	p, err := resolve(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return &Resolved{path: p, file: f}, nil
}
