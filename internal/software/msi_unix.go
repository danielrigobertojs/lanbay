// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import "errors"

func ReadMSIProductCode(path string) (string, error) {
	return "", errors.New("MSI no disponible en esta plataforma")
}
