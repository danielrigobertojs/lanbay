// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import "context"

// NewTrash is intentionally fail-closed until the IFileOperation implementation
// is available. The API must never turn an unconfirmed recycle operation into a
// permanent deletion.
func NewTrash() Trash { return unavailableTrash{} }

type unavailableTrash struct{}

func (unavailableTrash) Delete(context.Context, string) error { return ErrRecycleUnavailable }
