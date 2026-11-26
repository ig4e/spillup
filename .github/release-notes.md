**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.8

- **Every quota, reset and credit on the Accounts page**, like CLIProxyAPI's Management Center:
  - **Claude:** every limit (5-hour, 7-day, and per-model weekly limits), extra-usage spend against its monthly cap, and which products used the week.
  - **Codex:** the plan, all rate-limit windows including code review, the credit balance with message estimates, and available limit resets.
  - **Antigravity:** the plan and each model family's remaining quota.
  - **Gemini CLI:** each model's remaining quota.
  - Each limit shows a bar, what's left, and when it resets.

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
