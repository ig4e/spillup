package pty

import "os/exec"

// Shell is the user's interactive shell: PowerShell 7 when installed, else Windows PowerShell.
func Shell() []string {
	if _, err := exec.LookPath("pwsh.exe"); err == nil {
		return []string{"pwsh.exe", "-NoLogo"}
	}
	return []string{"powershell.exe", "-NoLogo"}
}
