// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
)

// AuthenticodeInfo summarizes the Authenticode signature of a file.
type AuthenticodeInfo struct {
	Signed  bool   `json:"signed"`
	Signer  string `json:"signer,omitempty"`
	Valid   bool   `json:"valid"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func VerifyAuthenticode(ctx context.Context, path string) (AuthenticodeInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return AuthenticodeInfo{}, err
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-AuthenticodeSignature -FilePath $env:LANBAY_FILE | Select-Object Status, StatusMessage, @{Name='Signer';Expression={$_.SignerCertificate.Subject}} | ConvertTo-Json -Compress")
	cmd.Env = append(os.Environ(), "LANBAY_FILE="+path)
	out, err := cmd.Output()
	if err != nil {
		return AuthenticodeInfo{Status: "unknown", Message: err.Error()}, nil
	}
	var raw struct {
		Status        string `json:"Status"`
		StatusMessage string `json:"StatusMessage"`
		Signer        string `json:"Signer"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		// PowerShell may return an array; try array.
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
			return AuthenticodeInfo{Status: "unknown", Message: err.Error()}, nil
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
	if info.Status != "Valid" && info.Signer == "" {
		info.Status = "Unsigned"
	}
	return info, nil
}

// WinVerifyTrust is exposed as a helper name; implementation uses the OS
// verification stack via PowerShell (which calls WinVerifyTrust internally).
func WinVerifyTrust(path string) (AuthenticodeInfo, error) {
	return VerifyAuthenticode(context.Background(), path)
}
