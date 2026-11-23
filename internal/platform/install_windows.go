// SPDX-License-Identifier: Apache-2.0
//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const scheduledTaskName = "Lanbay"

// IsWindows reports whether this build targets Windows.
func IsWindows() bool { return true }

// InstallScheduledTask registers the Lanbay elevated logon task per ADR-004.
func InstallScheduledTask(exePath string) error {
	script := `$ErrorActionPreference='Stop'
$exe='` + exePath + `'
$principal=New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Highest
$action=New-ScheduledTaskAction -Execute $exe
$trigger=New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$settings=New-ScheduledTaskSettingsSet -Hidden -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName '` + scheduledTaskName + `' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register scheduled task: %w: %s", err, string(out))
	}
	return nil
}

// UninstallScheduledTask removes the Lanbay scheduled task if present.
func UninstallScheduledTask() error {
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", scheduledTaskName, "/F")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Ignore "not found" errors.
		if !containsNotFound(string(out)) {
			return fmt.Errorf("delete scheduled task: %w: %s", err, string(out))
		}
	}
	return nil
}

// AddFirewallRule allows inbound TCP on the given port, Private profile only.
func AddFirewallRule(port int) error {
	_ = RemoveFirewallRule()
	cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=Lanbay HTTPS", "dir=in", "action=allow", "protocol=TCP",
		"localport="+strconv.Itoa(port), "profile=private", "enable=yes")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add firewall rule: %w: %s", err, string(out))
	}
	return nil
}

// RemoveFirewallRule deletes the Lanbay firewall rule if present.
func RemoveFirewallRule() error {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=Lanbay HTTPS")
	_ = cmd.Run()
	return nil
}

func containsNotFound(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "cannot find") || strings.Contains(low, "does not exist") || strings.Contains(low, "no tasks")
}
