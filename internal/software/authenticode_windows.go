// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const trustENoSignature = 0x800B0100

// AuthenticodeInfo summarizes the Authenticode signature of a file.
type AuthenticodeInfo struct {
	Signed  bool   `json:"signed"`
	Signer  string `json:"signer,omitempty"`
	Valid   bool   `json:"valid"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// VerifyAuthenticode verifies the Authenticode signature of the file at path.
// It retains a shared-read handle and verifies over that handle (INS-2).
func VerifyAuthenticode(ctx context.Context, path string) (AuthenticodeInfo, error) {
	f, err := openRetained(path)
	if err != nil {
		return AuthenticodeInfo{}, err
	}
	defer f.Close()
	return VerifyAuthenticodeHandle(ctx, f)
}

// WinVerifyTrust is an alias kept for compatibility.
func WinVerifyTrust(path string) (AuthenticodeInfo, error) {
	return VerifyAuthenticode(context.Background(), path)
}

// VerifyAuthenticodeHandle verifies the Authenticode signature using an already
// retained file handle, closing the TOCTOU window (INS-2).
func VerifyAuthenticodeHandle(ctx context.Context, f *os.File) (AuthenticodeInfo, error) {
	if f == nil {
		return AuthenticodeInfo{}, errors.New("nil file")
	}
	if _, err := f.Stat(); err != nil {
		return AuthenticodeInfo{}, err
	}

	handle := windows.Handle(f.Fd())
	fileInfo := windows.WinTrustFileInfo{
		Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})),
		File: handle,
	}
	if p, err := windows.UTF16PtrFromString(f.Name()); err == nil {
		fileInfo.FilePath = p
	}

	wtData := windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_WHOLECHAIN,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&fileInfo),
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		ProvFlags:                       windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	ret := windows.WinVerifyTrustEx(0, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &wtData)

	wtData.StateAction = windows.WTD_STATEACTION_CLOSE
	windows.WinVerifyTrustEx(0, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &wtData)

	info := AuthenticodeInfo{}
	switch {
	case ret == nil:
		info.Signed = true
		info.Valid = true
		info.Status = "Valid"
	case func() bool {
		var errno windows.Errno
		if errors.As(ret, &errno) {
			return int(errno) == trustENoSignature
		}
		return false
	}():
		info.Signed = false
		info.Valid = false
		info.Status = "Unsigned"
	default:
		hr := int(ret.(windows.Errno))
		switch hr {
		case 0x800B010E, 0x80092013, 0x80092012: // revocation unchecked
			info.Signed = true
			info.Valid = false
			info.Status = "RevocationUnchecked"
			info.Message = ret.Error()
		default:
			info.Signed = true
			info.Valid = false
			info.Status = "Invalid"
			info.Message = ret.Error()
		}
	}

	// Best-effort signer extraction via PowerShell.
	psInfo, err := verifyAuthenticodePowerShell(ctx, f.Name())
	if err == nil {
		if psInfo.Signer != "" {
			info.Signer = psInfo.Signer
		}
		if psInfo.Message != "" && info.Message == "" {
			info.Message = psInfo.Message
		}
		if psInfo.Status == "Unsigned" {
			info.Signed = false
			info.Valid = false
			info.Status = "Unsigned"
		}
	}
	if info.Status == "" {
		info.Status = "Unknown"
	}
	return info, nil
}

func verifyAuthenticodePowerShell(ctx context.Context, path string) (AuthenticodeInfo, error) {
	ps := filepath.Join(systemDirectory(), "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(ps); err != nil {
		// Omit signer extraction; do not fall back to PATH.
		return AuthenticodeInfo{}, nil
	}
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-Command",
		"Get-AuthenticodeSignature -FilePath $env:LANBAY_FILE | Select-Object Status, StatusMessage, @{Name='Signer';Expression={$_.SignerCertificate.Subject}} | ConvertTo-Json -Compress")
	cmd.Env = []string{
		"PATH=" + systemDirectory(),
		"SystemRoot=" + filepath.Dir(systemDirectory()),
		"LANBAY_FILE=" + path,
	}
	out, err := cmd.Output()
	if err != nil {
		return AuthenticodeInfo{}, err
	}
	var raw struct {
		Status        string `json:"Status"`
		StatusMessage string `json:"StatusMessage"`
		Signer        string `json:"Signer"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		var arr []map[string]any
		if err2 := json.Unmarshal(out, &arr); err2 == nil && len(arr) > 0 {
			if v, ok := arr[0]["Status"].(string); ok {
				raw.Status = v
			}
			if v, ok := arr[0]["StatusMessage"].(string); ok {
				raw.StatusMessage = v
			}
			if v, ok := arr[0]["Signer"].(string); ok {
				raw.Signer = v
			}
		} else {
			return AuthenticodeInfo{}, err
		}
	}
	info := AuthenticodeInfo{Status: raw.Status, Message: raw.StatusMessage, Signer: raw.Signer}
	if raw.Status == "Valid" {
		info.Signed = true
		info.Valid = true
	} else if raw.Signer != "" {
		info.Signed = true
	}
	if info.Status == "" {
		info.Status = "Unknown"
	}
	return info, nil
}
