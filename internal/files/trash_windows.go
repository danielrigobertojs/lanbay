// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import "context"

// NewTrash is kept behind the platform boundary. The IFileOperation-backed
// implementation is introduced with the Windows handle hardening work; until
// then the server fails closed instead of silently deleting permanently.
func NewTrash() Trash { return unavailableTrash{} }

type unavailableTrash struct{}

func (unavailableTrash) Delete(context.Context, string) error { return ErrRecycleUnavailable }
