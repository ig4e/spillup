import type { App, Device, Doctor, SetupProgress, Done, Keys, LoginStatus, Machines, Overview, OwnState, SignIn, SignInStatus, Skills, T3State, UpdateState } from "./types";

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
  const res = await fetch(`/api/${path}`, {
    method,
    headers: method === "POST" ? { "Content-Type": "application/json" } : undefined,
    body: method === "POST" ? JSON.stringify(body ?? {}) : undefined,
    credentials: "same-origin",
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(data?.error?.message ?? `${res.status} ${res.statusText}`, res.status);
  }
  return data as T;
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
  proxyLogins: () => call<LoginStatus>("GET", "proxy/logins"),
  t3: () => call<T3State>("GET", "t3"),
  t3Sync: () => post("t3/sync"),
  t3Setup: () => post("t3/setup"),
  scan: () => call<Device[]>("GET", "machines/scan"),
  setHubOnly: (hub_only: boolean) => post("accounts/mode", { hub_only }),
  fix: (fix: string) => post("fix", { fix }),
  setup: (name: string, role: string) => post("setup", { name, role }),
  setupProgress: () => call<SetupProgress>("GET", "setup"),
  update: () => call<UpdateState>("GET", "update"),
  startUpdate: () => post("update"),
  setAutoUpdate: (auto: boolean) => post("update/auto", { auto }),
};
