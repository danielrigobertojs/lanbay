// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

func newBackendWithDataDir(string) Backend {
	return NewDisabledBackend("software deshabilitado en esta plataforma")
}
