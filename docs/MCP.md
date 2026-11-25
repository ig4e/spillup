# MCP servers

Open **Skills & MCP → MCP** on the machine where you want to configure Claude Code or Codex. Add a stdio command or an HTTPS server, then enable it for each agent that should use it. Both clients register the server under `spillup-<name>`. Enabling a command lets the agent run it under your user account. Review the command and arguments first.

## Choose what travels

New MCP definitions start local. **Publish local copy** shares the executable name and arguments or HTTPS URL, along with the names of any required environment variables. Use **Keep local copy** to pause updates for one server on this machine. Other machines keep their shared version. You can publish your edits or switch back to the current shared definition later. **Withdraw shared definition** keeps a local copy and removes the definition from future fleet snapshots.

Publishing a definition never enables it in another agent. Approvals belong to a particular agent home on a particular machine. Edited stdio definitions require fresh approval before their next launch. For HTTPS, the client continues to use its last approved address until you update or disable the entry locally. Withdrawing a definition does not terminate a running server, remove an already registered HTTPS endpoint from another client, or revoke a credential at its provider.

## Secrets

For a stdio server, declare the uppercase variable names it reads, then use **Set secret** to enter each value locally. The launcher supplies those values to the approved process. Arguments and URL paths are ordinary definition text. Put credentials in the secret fields. HTTPS servers use the client's native authentication, including interactive OAuth where supported.

To send a value once, open the same definition on both machines. On the receiving machine choose the key, select the sending machine and choose **Allow**. On the sender choose the verified recipient and share that key within ten minutes. Both machines must be online. A grant applies to exactly one key, sender and recipient and is consumed on receipt. Future rotations require a new transfer. **Forget** removes the local value. Rotate the credential with its provider to invalidate copies already delivered.

Vault files stay in the current user's private Spillup state directory. The vault stores recoverable values protected by operating-system file permissions. People with a shell or remote desktop running as that OS user have the same access. Backups of the user profile may include them. The dashboard lists the available key names. It has no endpoint that reads a saved value back.

## CLI

For example, save a portable definition in a file called `notes.json`. Install the server executable separately on each machine where it will run. On Windows use a native executable, or explicitly select the interpreter that runs a trusted script.

```json
{"name":"notes","transport":"stdio","command":"notes-mcp","secretKeys":["NOTES_TOKEN"]}
```

```sh
spinup mcp import notes.json
spinup mcp secret set notes NOTES_TOKEN
spinup mcp enable notes codex
spinup mcp sharing notes publish
spinup mcp list
```

`spinup mcp secret set` accepts a hidden terminal prompt or standard input. For a one-time transfer run `spinup mcp secret allow notes NOTES_TOKEN <sender>` on the receiver, then `spinup mcp secret share notes NOTES_TOKEN <recipient>` on the sender. To disable the server in a native agent, use `spinup mcp disable notes codex`. Use `spinup mcp remove notes` after disabling its local registrations.

The clients support stdio and Streamable HTTP. Consult their documentation for authentication and native MCP controls. [MCP transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) · [Claude Code MCP](https://code.claude.com/docs/en/mcp) · [Codex MCP](https://developers.openai.com/codex/mcp/)
