package sshd

import (
	"context"
	"os/exec"
	"strings"

	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// On reports whether Windows' OpenSSH server runs here.
func On(ctx context.Context, _ string) bool {
	cmd := exec.CommandContext(ctx, "sc.exe", "query", "sshd")
	sysproc.Hide(cmd)
	out, err := cmd.Output()
	return err == nil && strings.Contains(string(out), "RUNNING")
}

// enableScript installs and starts Windows' OpenSSH server at boot, lets in only the tailnet, opens
// PowerShell, and reads keys from each user's own authorized_keys, administrators too, so spillup keeps
// them without admin.
const enableScript = `
$ErrorActionPreference = 'Stop'
if (-not (Get-Service sshd -ErrorAction SilentlyContinue)) {
  Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0 | Out-Null
}
$config = "$env:ProgramData\ssh\sshd_config"
Start-Service sshd
if (Test-Path $config) {
  $text = Get-Content $config -Raw
  $text = $text -replace '(?m)^(Match Group administrators)', '#$1' -replace '(?m)^(\s+AuthorizedKeysFile __PROGRAMDATA__)', '#$1'
  Set-Content $config $text -Encoding ascii
}
$shell = (Get-Command pwsh.exe -ErrorAction SilentlyContinue).Source
if (-not $shell) { $shell = "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" }
New-Item -Path 'HKLM:\SOFTWARE\OpenSSH' -Force | Out-Null
New-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -Name DefaultShell -Value $shell -PropertyType String -Force | Out-Null
Get-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (tailnet only, spillup)' -Direction Inbound -Protocol TCP -LocalPort 22 -RemoteAddress 100.64.0.0/10 -Action Allow | Out-Null
Set-Service sshd -StartupType Automatic
Restart-Service sshd
`

const disableScript = `
$ErrorActionPreference = 'Stop'
Stop-Service sshd -ErrorAction SilentlyContinue
Set-Service sshd -StartupType Disabled -ErrorAction SilentlyContinue
`

// Enable turns Windows' OpenSSH server on or off, which asks for admin.
func Enable(ctx context.Context, _ string, on bool) error {
	if on {
		return shell.Elevated(ctx, "Turn on SSH", enableScript)
	}
	return shell.Elevated(ctx, "Turn off SSH", disableScript)
}
