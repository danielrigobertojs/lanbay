// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

import "context"

type unavailableTrash struct{}

func NewTrash() Trash { return unavailableTrash{} }

func (unavailableTrash) Delete(context.Context, string) error { return ErrRecycleUnavailable }
