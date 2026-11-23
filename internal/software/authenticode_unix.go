// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import (
	"context"
	"errors"
	"os"
)

type AuthenticodeInfo struct {
	Signed  bool   `json:"signed"`
	Signer  string `json:"signer,omitempty"`
	Valid   bool   `json:"valid"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func VerifyAuthenticode(ctx context.Context, path string) (AuthenticodeInfo, error) {
	return AuthenticodeInfo{Signed: false, Valid: false, Status: "Unsigned", Message: "Authenticode solo disponible en Windows"}, nil
}

func VerifyAuthenticodeHandle(ctx context.Context, f *os.File) (AuthenticodeInfo, error) {
	if f == nil {
		return AuthenticodeInfo{}, errors.New("nil file")
	}
	return AuthenticodeInfo{Signed: false, Valid: false, Status: "Unsigned", Message: "Authenticode solo disponible en Windows"}, nil
}

func WinVerifyTrust(path string) (AuthenticodeInfo, error) {
	return VerifyAuthenticode(context.Background(), path)
}
