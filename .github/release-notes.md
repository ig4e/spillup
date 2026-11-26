**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.9

- **Refetch quotas on demand.** A Refetch button on the Accounts page has the account holder read every provider again right away (at most once every 30 seconds, so the accounts aren't flooded).
- **Every account shows its plan.** Claude (for example "Max 20x"), Codex, and Antigravity all show their plan.
- **Limits follow what each provider reports.** Accounts without a 5-hour window show only their weekly limits.

### In v0.4.8

- **Every quota, reset and credit on the Accounts page**, like CLIProxyAPI's Management Center:
  - **Claude:** every limit and per-model weekly limits, extra-usage spend against its cap, and which products used the week.
  - **Codex:** all rate-limit windows including code review, the credit balance with message estimates, and available limit resets.
  - **Antigravity and Gemini CLI:** each model's remaining quota.

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
