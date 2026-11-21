// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

func newBackend() Backend {
	return NewDisabledBackend("software deshabilitado en esta plataforma")
}
