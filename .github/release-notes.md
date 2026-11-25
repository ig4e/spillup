**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.5

- The phone navigation bar has more room above and below its buttons, and the large title sits further from them.
- `ssh <machine>` trusts a spillup machine's host key the first time it connects. Scripts and agents that run SSH non-interactively no longer fail with "Host key verification failed".

### Install

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ig4e/spillup/master/scripts/install.ps1 | iex
```

Linux (Arch and others) and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/ig4e/spillup/master/scripts/install.sh | sh
```

Or download the binary for your system below (`spinup_windows_amd64.exe`, `spinup_linux_amd64`, …) and check it against `checksums.txt`.
