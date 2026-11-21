// SPDX-License-Identifier: Apache-2.0
package software

import (
	"encoding/json"
	"fmt"
	"strings"
)

// expectedWingetSourceArg is the only acceptable URL for the built-in
// "winget" source (see SW-3).
const expectedWingetSourceArg = "https://cdn.winget.microsoft.com/cache"

// wingetSourceExport mirrors the JSON emitted by `winget source export`.
type wingetSourceExport struct {
	Name       string   `json:"Name"`
	Arg        string   `json:"Arg"`
	Type       string   `json:"Type"`
	Identifier string   `json:"Identifier"`
	Explicit   bool     `json:"Explicit"`
	TrustLevel []string `json:"TrustLevel"`
}

// ParseWingetSourceExport parses the JSON output of `winget source export`.
func ParseWingetSourceExport(output string) (wingetSourceExport, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return wingetSourceExport{}, fmt.Errorf("salida vacía de winget source export")
	}
	// Extract the first JSON object in case winget prints extra text.
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end < start {
		return wingetSourceExport{}, fmt.Errorf("winget source export: JSON no reconocido")
	}
	var s wingetSourceExport
	if err := json.Unmarshal([]byte(output[start:end+1]), &s); err != nil {
		return wingetSourceExport{}, fmt.Errorf("winget source export: %w", err)
	}
	return s, nil
}

// VerifyWingetSourceExport checks that the "winget" source points at the
// official CDN (SW-3). Returns nil on success.
func VerifyWingetSourceExport(output string) error {
	s, err := ParseWingetSourceExport(output)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(s.Name), "winget") {
		return fmt.Errorf("fuente inesperada en winget source export: %q", s.Name)
	}
	if strings.TrimSpace(s.Arg) != expectedWingetSourceArg {
		return fmt.Errorf("la fuente winget no apunta al CDN oficial: %q", s.Arg)
	}
	return nil
}

// VerifyWingetSettingsExport checks that InstallerHashOverride and
// LocalManifestFiles are not enabled in the winget settings (SW-3).
// Returns nil on success.
func VerifyWingetSettingsExport(output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		// Empty settings means defaults (both disabled) -> acceptable.
		return nil
	}
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end < start {
		// Not JSON; treat as unverifiable rather than a violation.
		return fmt.Errorf("winget settings export: JSON no reconocido")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(output[start:end+1]), &m); err != nil {
		return fmt.Errorf("winget settings export: %w", err)
	}
	var violations []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				kl := strings.ToLower(strings.TrimSpace(k))
				if kl == "installerhashoverride" || kl == "localmanifestfiles" {
					if isTruthy(val) {
						violations = append(violations, k)
					}
				}
				walk(val)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(m)
	if len(violations) > 0 {
		return fmt.Errorf("configuración insegura de winget: %s habilitado", strings.Join(violations, ", "))
	}
	return nil
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		l := strings.ToLower(strings.TrimSpace(t))
		return l == "true" || l == "enabled" || l == "1" || l == "yes" || l == "on"
	case float64:
		return t != 0
	case nil:
		return false
	default:
		// Non-nil objects/arrays count as enabled.
		return true
	}
}

// ValidateResultSource ensures a source field reported by winget is either
// empty or "winget". Any other source is rejected (SW-3).
func ValidateResultSource(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.EqualFold(s, string(SourceWinget)) {
		return nil
	}
	return fmt.Errorf("fuente no permitida: %q", s)
}
