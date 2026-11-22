// SPDX-License-Identifier: Apache-2.0
package software

import "testing"

func TestDecideAuthenticode(t *testing.T) {
	cases := []struct {
		name    string
		info    AuthenticodeInfo
		opts    InstallUploadOptions
		wantErr bool
	}{
		{"valid", AuthenticodeInfo{Signed: true, Valid: true, Status: "Valid"}, InstallUploadOptions{Path: "a.exe"}, false},
		{"unsigned no confirm", AuthenticodeInfo{Signed: false, Valid: false, Status: "Unsigned"}, InstallUploadOptions{Path: "a.exe"}, true},
		{"unsigned confirm", AuthenticodeInfo{Signed: false, Valid: false, Status: "Unsigned"}, InstallUploadOptions{Path: "a.exe", ConfirmUnsigned: true, ConfirmName: "a.exe"}, false},
		{"revocation unchecked confirm", AuthenticodeInfo{Signed: true, Valid: false, Status: "RevocationUnchecked"}, InstallUploadOptions{Path: "a.exe", ConfirmUnsigned: true, ConfirmName: "a.exe"}, false},
		{"revocation unchecked no confirm", AuthenticodeInfo{Signed: true, Valid: false, Status: "RevocationUnchecked"}, InstallUploadOptions{Path: "a.exe"}, true},
		{"invalid", AuthenticodeInfo{Signed: true, Valid: false, Status: "Invalid"}, InstallUploadOptions{Path: "a.exe", ConfirmUnsigned: true, ConfirmName: "a.exe"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideAuthenticode(tc.info, tc.opts)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected: %v", err)
			}
		})
	}
}
