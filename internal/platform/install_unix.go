// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

import "errors"

func IsWindows() bool { return false }

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
