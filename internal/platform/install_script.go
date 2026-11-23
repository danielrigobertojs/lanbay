// SPDX-License-Identifier: Apache-2.0
// Package platform provides OS-specific helpers. This file is build-neutral so
// the script construction can be unit-tested on any platform.
package platform

import "strings"

// scheduledTaskScript builds the PowerShell snippet that registers the Lanbay
// scheduled task. exePath is embedded as a single-quoted PowerShell string, with
// any embedded single quote doubled per PowerShell's escaping rules, so a path
// containing an apostrophe cannot break out of the string.
func scheduledTaskScript(exePath string) string {
	escaped := strings.ReplaceAll(exePath, "'", "''")
	return "$ErrorActionPreference='Stop'\n" +
		"$exe='" + escaped + "'\n" +
		"$principal=New-ScheduledTaskPrincipal -UserId \"$env:USERDOMAIN\\$env:USERNAME\" -LogonType Interactive -RunLevel Highest\n" +
		"$action=New-ScheduledTaskAction -Execute $exe\n" +
		"$trigger=New-ScheduledTaskTrigger -AtLogOn -User \"$env:USERDOMAIN\\$env:USERNAME\"\n" +
		"$settings=New-ScheduledTaskSettingsSet -Hidden -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries\n" +
		"Register-ScheduledTask -TaskName 'Lanbay' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null"
}
