# Your library: `~/.spinup`

Your skills list, skills and instructions live in one folder on each machine. Spinup syncs those parts of the folder between your machines. There is no repo to clone, no GitHub account, nothing to pull.

```
~/.spinup/
  skills.json       your skills list
  skills/<name>/    your own skills: a folder with a SKILL.md each
  AGENTS.md         your instructions, added after spinup's
  fetched/<name>/   a copy of each skill on your list, fetched from GitHub once
  mcp-servers.json  explicitly shared MCP definitions
  packages.json     requested plugin packages and their locked revisions
  artifacts/        verified package archives, transferred separately by digest
  local/            this machine's package catalogs, content and installation receipts
```

Everything is optional. Without a library, spinup uses its suggested skills and instructions.

## How it fills

- **Skills list:** it starts as spinup's suggested list. Your first `spinup skills add`, `remove`, `manual` or `auto` copies that list into `skills.json` and changes the copy; from then on your list is the one used.
- **Own skills:** `spinup skills new <name>` starts one, `spinup skills import <path|zip>` takes one someone sent you. Both write a folder with a `SKILL.md` into `skills/` and install it for Claude Code and Codex. Dropping a folder in by hand and running `spinup skills sync` does the same.
- **Instructions:** write `AGENTS.md`, then run `spinup agents`. It is added after spinup's instructions in `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`.
- **Packages:** `spinup packages add owner/repo` resolves and installs a complete plugin. Its locked revision and requested tools are shared; native installation state stays local. See [PACKAGES.md](PACKAGES.md).

`SPINUP_LIBRARY` moves the library to another folder.

## Shared between your machines

Change your library on any machine (a `spinup skills` command, or by hand) and within about 15 seconds every other machine has the change and has installed it: the skills for Claude Code and Codex, and your instructions.

- **Through the hub or standby.** The spinup service on each machine compares libraries with the machine holding the accounts, and the hub and standbys compare with each other. So a change reaches everyone as long as a hub or standby is on.
- **The newest change wins.** If two machines change the library at nearly the same time, the later change is kept on all machines.
- **Commands start from the newest.** `spinup skills` commands first bring in a newer library from your other machines, so your change builds on it. You never wait for the 15 seconds: the command pulls straight away.
- **A new machine never overwrites yours.** A library you never changed loses to any library you did, so a new machine takes yours.
- **Skills are fetched once.** The machine where you add a skill fetches it from GitHub (with Node.js) and keeps a copy in `fetched/`; the other machines install that copy, without Node.js or GitHub.
- **Only your machines.** Libraries travel inside your tailnet, and only to and from machines that know your API key.

## What stays on each machine

Library snapshots send only `AGENTS.md`, `skills.json`, `skills/`, `fetched/`, `packages.json` and `mcp-servers.json`. Hidden files and symbolic links remain excluded from these snapshots. Verified package archives are transferred separately by digest and preserve hidden plugin manifests inside the archive. An accepted update preserves files outside the shared paths, including local package installation receipts and catalogs.

Portable MCP definitions can be shared individually. Agent approvals, private definitions and credential values stay in machine-local state. A named credential can be sent to a verified machine with one-time approval on the recipient. [MCP guide](MCP.md)

Account sharing has its own service. [Account sharing](SERVICE.md)

Shared instructions and skill files are copied as content: spinup does not detect secrets or rewrite machine-specific values inside them. Keep these files portable and refer to local configuration instead of embedding device details.

Package sync uses an explicit portable declaration and locked artifact, then reconciles native installation on each machine. It does not copy tool configuration, credentials or MCP connection state. Unknown configuration is not automatically portable. All participating machines need package-aware Spinup before adding packages; legacy whole-library replacements are refused once `packages.json` exists.

## Per-skill sharing

In **Skills & MCP**, choose **Keep local copy** on a skill to work on a private copy. This machine keeps using it through later library syncs. The last published copy remains available to other machines. Use **Use shared copy** to archive your changes locally and install the current shared copy, or **Publish local copy** to share your working copy.

New or imported skills can start private. For example, `spinup skills new notes --private`, `spinup skills import ./notes --private` and `spinup skills add owner/repo notes --private` keep the new skill on this machine from the outset. For an existing skill use `spinup skills sharing notes private`, then `resume` or `publish` when you want to rejoin the fleet. A private working copy cannot recall content already sent to other machines.

MCP servers have a separate setting for sharing their definitions. Update all participating machines before publishing the first MCP definition. [MCP servers](MCP.md)

## Coming from the private repo

Earlier versions kept these files in a private git repo cloned into `local/`. The first time spinup runs from a checkout that has one, it copies `skills.json`, `skills/` and `AGENTS.md` into the library (never over what is already there). After that `local/` is not used and can go.

## Examples for `AGENTS.md`

```markdown
## Personal
- Reply in English, even when I write in another language.
- My projects live in ~/work and ~/personal.
```

Keep it short: it loads in every session.
