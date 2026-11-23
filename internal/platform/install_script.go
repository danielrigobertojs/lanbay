// SPDX-License-Identifier: Apache-2.0
// Package platform provides OS-specific helpers. This file is build-neutral so
// the script construction can be unit-tested on any platform.
package platform

// scheduledTaskScript builds the PowerShell snippet that registers the Lanbay
// scheduled task. The executable path is NOT embedded in the script: it is
// passed through the LANBAY_EXE environment variable, so neither quoting nor
// typographic-quote escaping can break out of a string.
func scheduledTaskScript() string {
	return "$ErrorActionPreference='Stop'\n" +
		"$exe=$env:LANBAY_EXE\n" +
		"if ([string]::IsNullOrWhiteSpace($exe)) { throw 'LANBAY_EXE is not set' }\n" +
		"$principal=New-ScheduledTaskPrincipal -UserId \"$env:USERDOMAIN\\$env:USERNAME\" -LogonType Interactive -RunLevel Highest\n" +
		"$action=New-ScheduledTaskAction -Execute $exe\n" +
		"$trigger=New-ScheduledTaskTrigger -AtLogOn -User \"$env:USERDOMAIN\\$env:USERNAME\"\n" +
		"$settings=New-ScheduledTaskSettingsSet -Hidden -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries\n" +
		"Register-ScheduledTask -TaskName 'Lanbay' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null"
}
