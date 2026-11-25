import { call, read, type DashboardRead } from "./api";

export type MCPTarget = "claude" | "codex";

export type Sharing = "local" | "shared";
export type SharingAction = "detach" | "publish" | "follow" | "unpublish";

export interface MCPDefinition {
  sharing: Sharing;
  name: string;
  transport: "stdio" | "http";
  command?: string;
  args?: string[];
  url?: string;
  secretKeys?: string[];
}

export interface MCPServer extends MCPDefinition {
  hasShared: boolean;
  removed?: boolean;
  availableKeys: string[];
  enabled: MCPTarget[];
  changed?: boolean;
  changedTargets?: MCPTarget[];
  revision: string;
}

interface MCPState {
  availableTargets: MCPTarget[];
  servers: MCPServer[];
}

const demo = import.meta.env.VITE_MOCK === "1";
const demoServers = new Map<string, MCPServer>();
const sharedCopies = new Map<string, MCPDefinition>();
const approved = new Map<string, string>();
const revisions = new Map<string, string>();
const demoKeys = new Map<string, Set<string>>();
const live = read<MCPState>("mcp", true);
const sample = async (): Promise<MCPState> => ({
  availableTargets: ["claude", "codex"],
  servers: Array.from(demoServers.values(), s => structuredClone(s)),
});

export const mcpServers: DashboardRead<MCPState> = demo
  ? Object.assign(sample, { queryPath: live.queryPath, fetchPath: sample })
  : live;

const post = (route: string, body: unknown) => call<unknown>("POST", "mcp/" + route, body, true);

export async function saveServer(def: MCPDefinition) {
  if (!demo) return post("save", def);
  const prev = demoServers.get(def.name);
  if (def.sharing === "shared") sharedCopies.set(def.name, structuredClone(def));
  const signature = JSON.stringify([def.name, def.transport, def.command, def.args ?? [], def.url, def.secretKeys ?? []]);
  if (!revisions.has(signature)) revisions.set(signature, String(revisions.size + 1));
  const revision = revisions.get(signature)!;
  const enabled = prev?.enabled ?? [];
  const changedTargets = enabled.filter(target => approved.get(def.name + "/" + target) !== revision);
  demoServers.set(def.name, { ...def, hasShared: sharedCopies.has(def.name), revision, enabled, changedTargets,
    changed: changedTargets.length > 0, availableKeys: (def.secretKeys ?? []).filter(key => demoKeys.get(def.name)?.has(key)) });
}

export async function removeServer(name: string) {
  if (!demo) return post("remove", { name });
  const server = demoServers.get(name);
  if (server?.enabled.length) throw new Error("Disable each client first.");
  if (server?.sharing === "local" && sharedCopies.has(name)) await saveServer(sharedCopies.get(name)!);
  else { sharedCopies.delete(name); demoServers.delete(name); }
  approved.delete(name + "/claude");
  approved.delete(name + "/codex");
}

export async function setServerSharing(server: MCPServer, action: SharingAction) {
  if (!demo) return post("sharing", { name: server.name, revision: server.revision, action });
  const current = demoServers.get(server.name);
  if (!current || current.revision !== server.revision) throw new Error("The definition changed. Reopen it first.");
  const definition = action === "follow" ? sharedCopies.get(server.name) : current;
  if (action === "unpublish") sharedCopies.delete(server.name);
  if (!definition) throw new Error("The shared definition is unavailable.");
  const { name, command, args, secretKeys, transport, url } = definition;
  await saveServer({ name, command, args, secretKeys, transport, url, sharing: action === "detach" || action === "unpublish" ? "local" : "shared" });
}

export async function configureTarget(server: MCPServer, target: MCPTarget, enabled: boolean) {
  if (!demo) return post("target", { name: server.name, target, enabled, revision: server.revision });
  const current = demoServers.get(server.name);
  if (!current || current.revision !== server.revision) throw new Error("The definition changed. Reopen it first.");
  current.enabled = current.enabled.filter(t => t !== target);
  if (enabled) {
    current.enabled.push(target);
    approved.set(server.name + "/" + target, current.revision);
  } else approved.delete(server.name + "/" + target);
  current.changedTargets = current.enabled.filter(t => approved.get(server.name + "/" + t) !== current.revision);
  current.changed = current.changedTargets.length > 0;
}

export async function saveSecret(server: string, key: string, value: string) {
  if (!demo) return post("secret", { server, key, value });
  const current = demoServers.get(server);
  if (!current?.secretKeys?.includes(key)) throw new Error("Declare the key first.");
  if (!demoKeys.has(server)) demoKeys.set(server, new Set());
  demoKeys.get(server)!.add(key);
  if (!current.availableKeys.includes(key)) current.availableKeys.push(key);
}

export async function forgetSecret(server: string, key: string) {
  if (!demo) return post("forget", { server, key });
  const current = demoServers.get(server);
  demoKeys.get(server)?.delete(key);
  if (current) current.availableKeys = current.availableKeys.filter(k => k !== key);
}

export async function transferSecret(action: "allow" | "share", server: string, key: string, machine: string) {
  if (demo) throw new Error("Open the installed dashboard to transfer a credential between machines.");
  return post(action, { server, key, machine });
}
