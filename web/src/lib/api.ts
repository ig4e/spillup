import type { ActivityEntry, App, Device, SSHState, RemoteState, Stats, Doctor, SetupProgress, Done, Keys, LoginStatus, Machines, Overview, OwnState, SignIn, SignInStatus, Skills, T3State, UpdateState } from "./types";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

const mocked = import.meta.env.VITE_MOCK === "1";

// The machine whose dashboard this one shows; null is this machine. Its calls travel through this
// machine's service, which adds the keys.
let remote: string | null = new URLSearchParams(window.location.search).get("machine");
const remoteListeners = new Set<() => void>();

export function setRemote(machine: string | null) {
  remote = machine;
  const url = new URL(window.location.href);
  if (machine) url.searchParams.set("machine", machine);
  else url.searchParams.delete("machine");
  window.history.replaceState(null, "", url);
  remoteListeners.forEach((l) => l());
}

export function subscribeRemote(l: () => void) {
  remoteListeners.add(l);
  return () => {
    remoteListeners.delete(l);
  };
}

export const currentRemote = () => remote;

const routed = (path: string, local?: boolean) => (remote && !local ? `remote/${encodeURIComponent(remote)}/${path}` : path);

async function call<T>(method: "GET" | "POST", target: string, body?: unknown, local?: boolean): Promise<T> {
  const path = routed(target, local);
  if (mocked) {
    const { mock } = await import("./mock");
    return mock(method, path, body) as Promise<T>;
  }
  const timeout = target === "setup" && method === "GET" ? 8_000 : method === "GET" ? 45_000 : 90_000;
  const signal = AbortSignal.timeout(timeout);
  try {
    const res = await fetch(`/api/${path}`, {
      method,
      headers: method === "POST" ? { "Content-Type": "application/json" } : undefined,
      body: method === "POST" ? JSON.stringify(body ?? {}) : undefined,
      credentials: "same-origin",
      signal,
    });
    if (res.status === 204) return undefined as T;
    const data = res.headers.get("Content-Type")?.includes("application/json") ? await res.json() : undefined;
    if (!res.ok) {
      throw new ApiError(data?.error?.message ?? `${res.status} ${res.statusText}`, res.status);
    }
    if (data === undefined) throw new ApiError("The machine returned an unexpected response.", res.status);
    return data as T;
  } catch (error) {
    if (signal.aborted) {
      throw new ApiError(method === "GET" ? "The machine took too long to respond." : "The machine took too long to respond. Check its status before retrying.", 408);
    }
    throw error;
  }
}

const post = (path: string, body?: unknown) => call<Done>("POST", path, body);

export const api = {
  overview: () => call<Overview>("GET", "overview"),
  machines: () => call<Machines>("GET", "machines"),
  localMachines: () => call<Machines>("GET", "machines", undefined, true),
  localOverview: () => call<Overview>("GET", "overview", undefined, true),
  doctor: () => call<Doctor>("GET", "doctor"),
  skills: () => call<Skills>("GET", "skills"),
  keys: () => call<Keys>("GET", "keys"),

  handoff: (to: string) => post("accounts/handoff", { to }),
  takeover: () => post("accounts/takeover"),
  restartProxy: () => post("accounts/restart"),
  makeMain: (machine: string) => post("machines/main", { machine }),

  setMode: (names: string[], manual: boolean) => post("skills/mode", { names, manual }),
  removeSkills: (names: string[]) => post("skills/remove", { names }),
  restoreSkill: (name: string) => post("skills/restore", { name }),
  adoptSkills: (names: string[]) => post("skills/adopt", { names }),
  addSkills: (repo: string, names: string[], manual: boolean) => post("skills/add", { repo, names, manual }),
  syncSkills: () => post("skills/sync"),

  own: () => call<OwnState>("GET", "own"),
  setOwn: (providers: string[]) => post("own", { providers }),
  signIn: (provider: string) => call<SignIn>("POST", "own/signin", { provider }),
  signInStatus: (state: string) => call<SignInStatus>("GET", `own/signin?state=${encodeURIComponent(state)}`),
  signOut: (name: string) => post("own/signout", { name }),

  apps: () => call<App[]>("GET", "apps"),
  remote: () => call<RemoteState>("GET", "remote", undefined, true),
  /** A host's apps, from this machine's moonlight-web, which the dashboard serves at /remote/. */
  remoteApps: async (hostId: number): Promise<{ app_id: number; title: string }[]> => {
    if (mocked) return [{ app_id: 1, title: "Desktop" }, { app_id: 2, title: "Steam Big Picture" }];
    const res = await fetch(`/remote/api/apps?host_id=${hostId}`, { credentials: "same-origin", signal: AbortSignal.timeout(15_000) });
    if (!res.ok) throw new ApiError((await res.text()) || res.statusText, res.status);
    return ((await res.json()) as { apps: { app_id: number; title: string }[] }).apps;
  },
  remoteSync: () => call<{ message: string }>("POST", "remote/sync", {}, true),
  remoteSetup: () => call<{ message: string }>("POST", "remote/setup", {}, true),
  /** Confirms the PIN Moonlight shows on machine's Sunshine (null: this machine). */
  sunshinePin: (machine: string | null, pin: string) =>
    call<{ message: string }>("POST", machine ? `remote/${encodeURIComponent(machine)}/sunshine/pin` : "sunshine/pin", { pin }, true),
  recentActivity: () => call<ActivityEntry[]>("GET", "activity"),
  /** A machine's specs and load: this one, or another by name, whichever dashboard is shown. */
  statsOf: (machine: string | null) => call<Stats>("GET", machine ? `remote/${encodeURIComponent(machine)}/stats` : "stats", undefined, true),
  /** Asks a program on machine (null: this one) to quit, or with force ends it. */
  endProcess: (machine: string | null, pid: number, force: boolean) =>
    call<Done>("POST", machine ? `remote/${encodeURIComponent(machine)}/processes/end` : "processes/end", { pid, force }, true),
  proxyLogins: () => call<LoginStatus>("GET", "proxy/logins"),
  t3: () => call<T3State>("GET", "t3"),
  t3Sync: () => post("t3/sync"),
  t3Setup: () => post("t3/setup"),
  scan: () => call<Device[]>("GET", "machines/scan"),
  setHubOnly: (hub_only: boolean) => post("accounts/mode", { hub_only }),
  fix: (fix: string) => post("fix", { fix }),
  setup: (name: string, role: string) => post("setup", { name, role }),
  setupProgress: () => call<SetupProgress>("GET", "setup", undefined, true),
  answerSetup: (password: string, cancel = false) => post("setup/answer", { password, cancel }),
  update: () => call<UpdateState>("GET", "update"),
  startUpdate: () => post("update"),
  setAutoUpdate: (auto: boolean) => post("update/auto", { auto }),
  restart: () => post("restart"),
  updateSystem: () => post("update/system"),
  setAccess: (device_access: boolean, terminals: boolean) => post("settings/access", { device_access, terminals }),
  t3Pair: (label: string) => call<{ code: string }>("POST", "t3/pair", { label }),

  ssh: () => call<SSHState>("GET", "ssh"),
  sshEnable: (on: boolean) => post("ssh/enable", { on }),
  sshSync: () => post("ssh/sync"),

  termStart: (cols: number, rows: number) => call<{ id: string }>("POST", "term", { cols, rows }),
  /** The terminal's output as it comes: a stream of bytes, starting with what it showed recently. */
  termOutput: async (id: string, signal: AbortSignal): Promise<ReadableStream<Uint8Array>> => {
    const res = await fetch(`/api/${routed(`term/${id}/out`)}`, { credentials: "same-origin", signal });
    if (!res.ok || !res.body) {
      const data = await res.json().catch(() => ({}));
      throw new ApiError(data?.error?.message ?? `${res.status} ${res.statusText}`, res.status);
    }
    return res.body;
  },
  termInput: (id: string, data: string) => call<void>("POST", `term/${id}/in`, { data }),
  termResize: (id: string, cols: number, rows: number) => call<void>("POST", `term/${id}/size`, { cols, rows }),
  termClose: (id: string) => call<void>("POST", `term/${id}/close`, {}),
};
