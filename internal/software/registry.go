// SPDX-License-Identifier: Apache-2.0
package software

// UninstallEntry describes one Windows "Uninstall" registry entry used to
// detect what an installer registered.
type UninstallEntry struct {
	Key                  string `json:"key"`
	Root                 string `json:"root,omitempty"`
	DisplayName          string `json:"display_name"`
	DisplayVersion       string `json:"display_version,omitempty"`
	Publisher            string `json:"publisher,omitempty"`
	InstallLocation      string `json:"install_location,omitempty"`
	QuietUninstallString string `json:"quiet_uninstall_string,omitempty"`
	UninstallString      string `json:"uninstall_string,omitempty"`
	ProductCode          string `json:"product_code,omitempty"`
}

// RegistrySnapshot maps a unique registry key path to its entry.
type RegistrySnapshot map[string]UninstallEntry

// DiffUninstallKeys returns the entries present in after but not in before.
// Only brand-new keys are considered; pre-existing keys are never reported.
func DiffUninstallKeys(before, after RegistrySnapshot) []UninstallEntry {
	var result []UninstallEntry
	for key, entry := range after {
		if _, ok := before[key]; ok {
			continue
		}
		if entry.Key == "" {
			entry.Key = key
		}
		result = append(result, entry)
	}
	return result
}

// Aliases for compatibility with alternative naming used by callers/tests.
func DiffUninstallRegistry(before, after RegistrySnapshot) []UninstallEntry {
	return DiffUninstallKeys(before, after)
}
func DiffRegistry(before, after RegistrySnapshot) []UninstallEntry {
	return DiffUninstallKeys(before, after)
}
func DiffUninstallEntries(before, after RegistrySnapshot) []UninstallEntry {
	return DiffUninstallKeys(before, after)
}
func RegistryDiff(before, after RegistrySnapshot) []UninstallEntry {
	return DiffUninstallKeys(before, after)
}

// Type aliases for compatibility.
type (
	UninstallKey           = UninstallEntry
	RegistryEntry          = UninstallEntry
	UninstallRegistryEntry = UninstallEntry
)
