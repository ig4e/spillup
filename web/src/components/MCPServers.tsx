import { Modal, toast } from "@heroui/react";
import { KeyRound, Plus, Server } from "lucide-react";
import { cloneElement, useId, useRef, useState, type FormEvent, type ReactElement } from "react";
import { Button } from "./Button";
import { Group, GroupSkeleton, IconTile, Row } from "./ui";
import { api, localDashboard, setRemote } from "../lib/api";
import { dashboardClient, dashboardKey } from "../lib/query-cache";
import { useQuery, useRemote } from "../lib/hooks";
import {
  configureTarget, forgetSecret, mcpServers, removeServer, saveSecret, saveServer, setServerSharing, transferSecret,
  type MCPDefinition, type MCPServer, type MCPTarget, type Sharing, type SharingAction,
} from "../lib/mcp";

const clientNames: Record<MCPTarget, string> = { claude: "Claude Code", codex: "Codex" };
const sharingLabels: Record<SharingAction, string> = { detach: "Keep local copy", publish: "Publish local copy", follow: "Use shared copy", unpublish: "Withdraw shared definition" };
const clients: MCPTarget[] = ["claude", "codex"];
const control = "mt-1 w-full min-w-0 rounded-lg border border-border bg-surface px-3 py-2 text-base text-foreground focus-visible:outline-accent";
const problemText = "break-words text-sm text-danger";
const describe = (s: MCPDefinition) => s.transport === "http" ? s.url : [s.command, ...(s.args ?? []).map(a => JSON.stringify(a))].join(" ");

type Dialog =
  | { kind: "edit"; server?: MCPServer }
  | { kind: "remove"; server: MCPServer }
  | { kind: "sharing"; server: MCPServer; action: SharingAction }
  | { kind: "approve"; server: MCPServer; target: MCPTarget }
  | { kind: "key"; server: MCPServer; key: string };

export function MCPServers() {
  const remote = useRemote();
  if (!localDashboard || remote) return (
    <Group>
      <Row title="Manage MCP on this machine" subtitle="Open the machine’s local dashboard with spinup ui to configure its AI clients and credentials." />
      {localDashboard && remote ? <Row title="Return to this machine" tone="accent" onPress={() => setRemote(null)} /> : null}
    </Group>
  );
  return <LocalServers />;
}

function LocalServers() {
  const q = useQuery(mcpServers, 15_000);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const open = (next: Dialog | null) => { setError(""); setDialog(next); };
  const change = async (action: () => Promise<unknown>, message: string, keyWrite = false) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await action();
      await dashboardClient.invalidateQueries({ queryKey: dashboardKey(mcpServers.queryPath(null)), exact: true });
      setDialog(null);
      toast.success(message);
    } catch (e) {
      setError(keyWrite ? "Couldn’t save this credential. Re-enter the value and check that the key is declared on this machine." : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  if (!q.data) {
    if (q.loading) return <GroupSkeleton rows={3} />;
    return <Group>
      <Row title="MCP management unavailable" subtitle={q.error ? q.error.message + " Open the local dashboard with spinup ui." : "Open an updated local dashboard with spinup ui."} />
      <Row title="Check again" tone="accent" onPress={() => void q.reload()} />
    </Group>;
  }
  const servers = q.data.servers ?? [];
  const installed = q.data.availableTargets ?? [];
  const dialogTitle = dialog?.kind === "edit" ? (dialog.server ? "Edit server" : "Add MCP server")
    : dialog?.kind === "sharing" ? sharingLabels[dialog.action]
    : dialog?.kind === "key" ? dialog.key : dialog?.kind === "remove" ? "Remove server?" : "Approve configuration";
  return <>
    <Group footer={q.refreshError ? "Refresh unavailable. Showing the last configuration." : "Each machine approves its own clients."}>
      <Row icon={<IconTile icon={Plus} color="blue" />} title="Add server" tone="accent" onPress={() => open({ kind: "edit" })} />
      {!servers.length ? <Row title="No MCP servers yet" /> : null}
    </Group>
    {!dialog && error ? <p role="alert" className={problemText}>{error}</p> : null}
    {servers.map(server => (
      <Group key={server.name} title={server.name}>
        <Row icon={<IconTile icon={Server} color="indigo" />} title={server.transport === "http" ? "HTTPS endpoint" : "Command"}
          subtitle={<code className="[overflow-wrap:anywhere] whitespace-pre-wrap">{describe(server)}</code>} />
        <Row title={server.sharing === "local" ? "Local definition" : "Shared definition"}
          subtitle={server.removed ? "Removed from the library. Disable the remaining local approvals." : server.sharing === "local" ? "Edits stay on this machine." : "Follows changes in the library."} />
        {!server.removed ? <div className="flex flex-wrap gap-2 px-3 py-2">
          {(server.sharing === "shared" ? ["detach", "unpublish"] as const : server.hasShared ? ["publish", "follow", "unpublish"] as const : ["publish"] as const).map(action =>
            <Button size="sm" variant="secondary" key={action} isDisabled={busy}
              aria-label={sharingLabels[action] + " for " + server.name}
              onPress={() => open({ kind: "sharing", server, action })}>{sharingLabels[action]}</Button>)}
        </div> : null}
        {clients.map(target => {
          const enabled = server.enabled?.includes(target) ?? false;
          const available = installed.includes(target);
          const changed = server.changedTargets?.includes(target) ?? server.changed;
          return <Row key={target} title={clientNames[target]}
            subtitle={!available ? (enabled ? "Approved here · client unavailable" : "Client unavailable") : enabled && changed ? "Review the changed definition" : enabled ? "Enabled here" : "Off on this machine"}
            accessory={<div className="flex flex-wrap justify-end gap-1">
              {!server.removed && available && (!enabled || changed) ? <Button size="sm" variant="secondary" isDisabled={busy}
                aria-label={`${enabled ? "Review" : "Enable"} ${server.name} in ${clientNames[target]}`}
                onPress={() => open({ kind: "approve", server, target })}>{enabled ? "Review" : "Enable"}</Button> : null}
              {enabled ? <Button size="sm" variant="tertiary" isDisabled={busy}
                aria-label={`Disable ${server.name} in ${clientNames[target]}`}
                onPress={() => void change(() => configureTarget(server, target, false), `Disabled in ${clientNames[target]}.`)}>Disable</Button> : null}
            </div>} />;
        })}
        {(server.removed ? server.availableKeys : server.secretKeys ?? [])?.map(key => <Row key={key} icon={<IconTile icon={KeyRound} color="orange" />}
          title={key} value={server.availableKeys?.includes(key) ? "Saved here" : "Add credential"}
          onPress={() => open({ kind: "key", server, key })} />)}
        {!server.removed ? <div className="flex flex-wrap items-center justify-end gap-2 px-3 py-2">
          {server.enabled?.length ? <span className="text-xs text-muted">Disable each client to remove this definition.</span> : null}
          <Button size="sm" variant="tertiary" aria-label={`Edit ${server.name}`} onPress={() => open({ kind: "edit", server })}>Edit</Button>
          <Button size="sm" variant="danger-soft" isDisabled={busy || !!server.enabled?.length} aria-label={`Remove ${server.name}`} onPress={() => open({ kind: "remove", server })}>Remove</Button>
        </div> : null}
      </Group>
    ))}
    {dialog ? <Modal.Backdrop isOpen onOpenChange={visible => { if (!visible && !busy) open(null); }} variant="blur">
      <Modal.Container size="sm">
        <Modal.Dialog>
          <Modal.Header><Modal.Heading className="[overflow-wrap:anywhere]">{dialogTitle}</Modal.Heading></Modal.Header>
          <Modal.Body className="min-w-0 overflow-y-auto">
            {dialog.kind === "edit" ? <DefinitionForm initial={dialog.server} names={servers.map(s => s.name)} busy={busy}
              save={def => change(() => saveServer(def), "Server definition saved.")} /> : null}
            {dialog.kind === "key" ? <CredentialForm server={dialog.server} name={dialog.key} busy={busy} change={change} /> : null}
            {dialog.kind === "sharing" ? <div className="space-y-4">
              <p>{dialog.action === "detach" ? "Keep the current definition on this machine, including when the shared copy changes."
                : dialog.action === "unpublish" ? "Keep this machine’s local definition and remove the shared copy from the library. Other machines with an approved HTTPS connection keep it until they disable it locally."
                : dialog.action === "publish" ? "Publish this definition and its plain-text arguments to your shared library. Local credentials stay on this machine."
                : "Replace this machine’s edits with the shared definition and follow future changes."}</p>
              <Button isPending={busy} onPress={() => void change(() => setServerSharing(dialog.server, dialog.action), "Sharing updated.")}>{sharingLabels[dialog.action]}</Button>
            </div> : null}
            {dialog.kind === "approve" ? <div className="space-y-3">
              <p>Enable <strong>{dialog.server.name}</strong> in {clientNames[dialog.target]} on this machine?</p>
              <pre className="rounded-lg bg-surface-secondary p-3 text-xs whitespace-pre-wrap [overflow-wrap:anywhere]">{describe(dialog.server)}</pre>
              {dialog.server.secretKeys?.length ? <p className="break-words text-sm text-muted">Uses credential keys {dialog.server.secretKeys.join(", ")}.</p> : null}
              {dialog.server.secretKeys?.some(key => !dialog.server.availableKeys?.includes(key)) ? <p className="text-sm text-warning">Save the missing credentials on this machine before starting the server.</p> : null}
              <Button isPending={busy} onPress={() => void change(() => configureTarget(dialog.server, dialog.target, true), `Enabled in ${clientNames[dialog.target]}.`)}>Approve and enable</Button>
            </div> : null}
            {dialog.kind === "remove" ? <div className="space-y-4">
              <p>Remove <strong>{dialog.server.name}</strong> from {dialog.server.sharing === "shared" ? "the shared library" : "this machine"}? {dialog.server.sharing === "local" && dialog.server.hasShared ? "The shared definition will be visible again." : ""}</p>
              {dialog.server.availableKeys?.length ? <p className="text-sm text-muted">Saved credentials stay on this machine. To remove them too, close this dialog and choose Forget locally for each key first.</p> : null}
              <Button variant="danger-soft" isPending={busy} onPress={() => void change(() => removeServer(dialog.server.name), "Server definition removed.")}>Remove server</Button>
            </div> : null}
            {error ? <p role="alert" className={"mt-3 " + problemText}>{error}</p> : null}
          </Modal.Body>
          <Modal.Footer><Button variant="tertiary" isDisabled={busy} onPress={() => open(null)}>Close</Button></Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop> : null}
  </>;
}

function Field({ label, children }: { label: string; children: ReactElement<{ id?: string }> }) {
  const id = useId();
  return <div><label htmlFor={id} className="block text-sm font-medium">{label}</label>{cloneElement(children, { id })}</div>;
}

function DefinitionForm({ initial, names, busy, save }: {
  initial?: MCPDefinition; names: string[]; busy: boolean; save: (definition: MCPDefinition) => Promise<void>;
}) {
  const [name, setName] = useState(initial?.name ?? "");
  const [transport, setTransport] = useState<MCPDefinition["transport"]>(initial?.transport ?? "stdio");
  const [sharing, setSharing] = useState<Sharing>(initial?.sharing ?? "local");
  const [command, setCommand] = useState(initial?.command ?? "");
  const [args, setArgs] = useState(initial?.args?.length ? JSON.stringify(initial.args) : "");
  const [keys, setKeys] = useState(initial?.secretKeys?.join(", ") ?? "");
  const [url, setURL] = useState(initial?.url ?? "");
  const [error, setError] = useState("");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError("");
    const id = name.trim();
    if (!/^[a-z][a-z0-9_-]{0,62}$/.test(id)) { setError("Use a lowercase name beginning with a letter, up to 63 characters."); return; }
    if (!initial && names.includes(id)) { setError("A server with this name already exists."); return; }
    if (transport === "http") {
      const endpoint = url.trim();
      let parsed: URL;
      try { parsed = new URL(endpoint); } catch { setError("Enter a valid HTTPS URL."); return; }
      if (parsed.protocol !== "https:" || parsed.username || parsed.password || /[?#]/.test(endpoint)) {
        setError("Use an HTTPS URL without credentials, a query or a fragment."); return;
      }
      void save({ name: id, sharing, transport, url: endpoint }); return;
    }
    let parsed: unknown;
    try { parsed = JSON.parse(args.trim() || "[]"); } catch { parsed = null; }
    if (!Array.isArray(parsed) || !parsed.every(arg => typeof arg === "string")) {
      setError('Use an array of quoted arguments, such as ["--stdio"].');
      return;
    }
    const secretKeys = [...new Set(keys.trim().split(/[\s,]+/).filter(Boolean))];
    if (secretKeys.some(key => !/^[A-Z_][A-Z0-9_]*$/.test(key))) { setError("Use uppercase environment variable names for credential keys."); return; }
    void save({ name: id, sharing, transport, command: command.trim(), args: parsed, secretKeys });
  };
  return <form onSubmit={submit} className="space-y-3">
    <Field label="Server name"><input className={control} required disabled={!!initial} value={name} onChange={e => setName(e.target.value)} autoComplete="off" /></Field>
    {!initial ? <Field label="Save definition to"><select className={control} value={sharing} onChange={e => setSharing(e.target.value as Sharing)}>
      <option value="local">This machine</option><option value="shared">Shared library</option>
    </select></Field> : null}
    <Field label="Transport"><select className={control} value={transport} onChange={e => setTransport(e.target.value as MCPDefinition["transport"])}>
      <option value="stdio">Local command (stdio)</option><option value="http">HTTPS</option>
    </select></Field>
    {transport === "stdio" ? <>
      <Field label="Command"><input className={control} required value={command} onChange={e => setCommand(e.target.value)} autoCapitalize="none" spellCheck={false} /></Field>
      <Field label="Arguments (JSON array)"><textarea className={control} rows={2} value={args} placeholder='["--stdio"]' onChange={e => setArgs(e.target.value)} autoCapitalize="none" spellCheck={false} /></Field>
      <p className="text-xs text-muted">Arguments are plain text and travel with a shared definition. Put credentials in the declared keys.</p>
      <Field label="Credential key names"><input className={control} value={keys} placeholder="SERVICE_TOKEN" onChange={e => setKeys(e.target.value)} autoComplete="off" spellCheck={false} /></Field>
    </> : <Field label="HTTPS endpoint"><input className={control} type="url" required value={url} onChange={e => setURL(e.target.value)} placeholder="https://example.com/mcp" autoCapitalize="none" spellCheck={false} /></Field>}
    {initial?.secretKeys?.some(key => transport !== "stdio" || !keys.split(/[\s,]+/).includes(key)) ? <p className="text-sm text-muted">Removing a key from the definition leaves its saved value on this machine. Choose Forget locally for that key first to delete the value.</p> : null}
    {error ? <p role="alert" className={problemText}>{error}</p> : null}
    <Button type="submit" isPending={busy} isDisabled={!name.trim() || !(transport === "http" ? url : command).trim()}>Save definition</Button>
  </form>;
}

type Change = (action: () => Promise<unknown>, message: string, keyWrite?: boolean) => Promise<void>;

function CredentialForm({ server, name, busy, change }: { server: MCPServer; name: string; busy: boolean; change: Change }) {
  const secret = useRef<HTMLInputElement>(null);
  const [forget, setForget] = useState(false);
  const [peer, setPeer] = useState("");
  const machines = useQuery(api.localManagedMachines, 15_000);
  const peers = (machines.data?.machines ?? []).filter(m => !m.is_self && m.online && !m.expired);
  const verified = !machines.refreshError && peers.some(m => m.name === peer);
  const saved = server.availableKeys?.includes(name) ?? false;
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!secret.current || !secret.current.value) return;
    const value = secret.current.value;
    secret.current.value = "";
    void change(() => saveSecret(server.name, name, value), "Credential saved on this machine.", true);
  };
  const forgetButton = saved ? <Button variant="danger-soft" isDisabled={busy} onPress={() => {
    if (forget) void change(() => forgetSecret(server.name, name), "Local credential removed.");
    else setForget(true);
  }}>{forget ? "Confirm removal" : "Forget locally"}</Button> : null;
  if (server.removed) return <div className="space-y-3"><p>The local vault still holds this key.</p>{forgetButton}</div>;
  return <div className="space-y-5">
    <form onSubmit={submit} className="space-y-2">
      <Field label={saved ? "Replace local credential" : "Local credential"}><input ref={secret} className={control} type="password" required autoComplete="new-password" spellCheck={false} autoCapitalize="none" /></Field>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" isPending={busy}>Save locally</Button>
        {forgetButton}
      </div>
    </form>
    <div className="space-y-2">
      <Field label="Transfer with one machine"><select className={control} value={verified ? peer : ""} onChange={e => setPeer(e.target.value)}>
        <option value="">Choose a managed machine</option>{peers.map(m => <option key={m.name} value={m.name}>{m.name}</option>)}
      </select></Field>
      <p className="text-xs text-muted">{machines.error || machines.refreshError ? "Machine list unavailable." : "On the receiving machine, allow the sender for ten minutes. Then share from the sender."}</p>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" isDisabled={!verified || busy} onPress={() => void change(() => transferSecret("allow", server.name, name, peer), `Transfers from ${peer} allowed for ten minutes.`)}>Allow from machine</Button>
        {saved ? <Button size="sm" variant="secondary" isDisabled={!verified || busy} onPress={() => void change(() => transferSecret("share", server.name, name, peer), `Credential shared with ${peer}.`)}>Share to machine</Button> : null}
      </div>
    </div>
  </div>;
}
