// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

import "errors"

func IsWindows() bool { return false }

func ProgramFilesDir() (string, error) { return "", errors.New("only supported on Windows") }
func InstallBinary(srcExe string) (string, error) {
	return "", errors.New("binary installation is only supported on Windows")
}
func RemoveInstallDir() error { return errors.New("only supported on Windows") }
func CheckInstallPathWritable(path string) error {
	return errors.New("only supported on Windows")
}

func InstallScheduledTask(exePath string) error {
	return errors.New("scheduled task installation is only supported on Windows")
}
func UninstallScheduledTask() error {
	return errors.New("scheduled task removal is only supported on Windows")
}
func AddFirewallRule(port int) error {
	return errors.New("firewall rule management is only supported on Windows")
}
func RemoveFirewallRule() error {
	return errors.New("firewall rule management is only supported on Windows")
}
func StopRunningInstance() {}
