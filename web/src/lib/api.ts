import { clearDeniedData, dashboardClient, dashboardKey, invalidateDashboard } from "./query-cache";
import type { FleetPreview, HealthObservation, JobRecord, QuotaSummary, T3Phones } from "./types";
import type { SetupPreview, ActivityEntry, App, Device, SSHState, RemoteState, Stats, Doctor, SetupProgress, Done, Keys, LoginStatus, Machines, Overview, OwnState, SignIn, SignInStatus, SkillSharingAction, Skills, T3State, UpdateState } from "./types";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

const hostname = window.location.hostname;
export const localDashboard = hostname === "localhost" || hostname === "[::1]" || /^127\./.test(hostname);

const mocked = import.meta.env.VITE_MOCK === "1";

// The machine whose dashboard this one shows; null is this machine. Its calls travel through this
// machine's service, which adds the keys.
const machineInURL = () => new URLSearchParams(window.location.search).get("machine") || null;
let remote = machineInURL();
const remoteListeners = new Set<() => void>();

function publishRemote(machine: string | null) {
  if (remote === machine) return;
  remote = machine;
  remoteListeners.forEach(listener => listener());
}

window.addEventListener("popstate", () => publishRemote(machineInURL()));

export function setRemote(machine: string | null) {
  const url = new URL(window.location.href);
  if (machine) url.searchParams.set("machine", machine);
  else url.searchParams.delete("machine");
  window.history.replaceState(window.history.state, "", url);
  publishRemote(machine);
}

export function subscribeRemote(l: () => void) {
  remoteListeners.add(l);
  return () => {
    remoteListeners.delete(l);
  };
}

export const currentRemote = () => remote;

export const queryPath = (path: string, machine: string | null) => machine ? `remote/${encodeURIComponent(machine)}/${path}` : path;
const routed = (path: string, local?: boolean) => queryPath(path, local ? null : remote);

export type DashboardRead<T> = (() => Promise<T>) & { queryPath: (machine: string | null) => string; fetchPath: (path: string) => Promise<T> };

export function read<T>(path: string, local = false, validate?: (data: T, previous?: T) => void): DashboardRead<T> {
  const key = (machine: string | null) => queryPath(path, local ? null : machine);
  const fetchPath = async (target: string) => {
    const data = await call<T>("GET", target, undefined, true);
    if (validate) validate(data, dashboardClient.getQueryData<T>(dashboardKey(target)));
    return data;
  };
  return Object.assign(() => fetchPath(key(currentRemote())), { queryPath: key, fetchPath });
}

function checkMachines(data: Machines) {
  if (data.problem && !data.machines?.length) throw new Error(data.problem);
}

function checkOverview(data: Overview, previous?: Overview) {
  if (data.installed && data.service_problem && !data.service && previous?.service) throw new Error(data.service_problem);
}

function checkRemote(data: RemoteState, previous?: RemoteState) {
  if (data.web_installed && !data.web_running && data.problem && previous?.web_running) throw new Error(data.problem);
}

async function responseError(path: string, res: Response): Promise<ApiError> {
  const contentType = res.headers.get("Content-Type") || "";
  let message = `${res.status} ${res.statusText}`;
  if (contentType.includes("application/json")) {
    const problem = await res.json().catch(() => null);
    if (typeof problem?.error?.message === "string") message = problem.error.message;
  } else if (contentType.includes("text/plain")) {
    const reason = (await res.text().catch(() => "")).trim();
    if (reason) message = reason.slice(0, 512);
  }
  const error = new ApiError(message, res.status);
  clearDeniedData(dashboardClient, path, error);
  return error;
}

export async function call<T>(method: "GET" | "POST", target: string, body?: unknown, local?: boolean): Promise<T> {
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
    if (!res.ok) throw await responseError(path, res);
    if (res.status === 204) return undefined as T;
    const data = res.headers.get("Content-Type")?.includes("application/json") ? await res.json() : undefined;
    if (data === undefined) throw new ApiError("The machine returned an unexpected response.", res.status);
    return data as T;
  } catch (error) {
    if (signal.aborted) {
      throw new ApiError(method === "GET" ? "The machine took too long to respond." : "The machine took too long to respond. Check its status before retrying.", 408);
    }
    throw error;
  }
}

async function post(path: string, body?: unknown, local?: boolean) {
  const result = await call<Done>("POST", path, body, local);
  void invalidateDashboard();
  return result;
}

export const api = {
  overview: read<Overview>("overview", false, checkOverview),
  machines: read<Machines>("machines", false, checkMachines),
  localMachines: read<Machines>("machines", true, checkMachines),
  managedMachines: read<Machines>("machines/managed", false, checkMachines),
  localManagedMachines: read<Machines>("machines/managed", true, checkMachines),
  localOverview: read<Overview>("overview", true, checkOverview),
  doctor: read<Doctor>("doctor"),
  skills: read<Skills>("skills"),
  keys: () => call<Keys>("GET", "keys"),

  sharedQuotas: read<QuotaSummary>("accounts/quotas"),
  handoff: (to: string) => post("accounts/handoff", { to }),
  takeover: () => post("accounts/takeover"),
  restartProxy: () => post("accounts/restart"),
  makeMain: (machine: string) => post("machines/main", { machine }),

  setMode: (machine: string | null, name: string, auto: boolean) =>
    call<Done>("POST", queryPath("skills/mode", machine), { names: [name], manual: !auto }, true),
  removeSkills: (names: string[]) => post("skills/remove", { names }),
  restoreSkill: (name: string) => post("skills/restore", { name }),
  adoptSkills: (names: string[]) => post("skills/adopt", { names }),
  addSkills: (repo: string, names: string[], manual: boolean, privateCopy = false) =>
    post("skills/add", privateCopy ? { repo, names, manual, private: true } : { repo, names, manual }, privateCopy),
  setSkillSharing: (name: string, action: SkillSharingAction) => post("skills/sharing", { name, action }, true),
  syncSkills: () => post("skills/sync"),

  own: read<OwnState>("own"),
  setOwn: (providers: string[]) => post("own", { providers }),
  signIn: (provider: string) => call<SignIn>("POST", "own/signin", { provider }),
  signInStatus: (state: string) => call<SignInStatus>("GET", `own/signin?state=${encodeURIComponent(state)}`, undefined, true),
  signOut: (name: string) => post("own/signout", { name }),

  apps: read<App[]>("apps"),
  remote: read<RemoteState>("remote", true, checkRemote),
  /** A host's apps, from this machine's moonlight-web, which the dashboard serves at /remote/. */
  remoteApps: async (hostId: number): Promise<{ app_id: number; title: string }[]> => {
    if (mocked) return [{ app_id: 1, title: "Desktop" }, { app_id: 2, title: "Steam Big Picture" }];
    const res = await fetch(`/remote/api/apps?host_id=${hostId}`, { credentials: "same-origin", signal: AbortSignal.timeout(15_000) });
    if (!res.ok) throw await responseError("stream/apps", res);
    return ((await res.json()) as { apps: { app_id: number; title: string }[] }).apps;
  },
  remoteClipboard: (hostId: number) => call<{ text: string }>("POST", "remote/clipboard", { host_id: hostId }, true),
  remoteSync: () => post("remote/sync", {}, true),
  remoteSetup: () => post("remote/setup", {}, true),
  /** Confirms the PIN Moonlight shows on machine's Sunshine (null: this machine). */
  sunshinePin: (machine: string | null, pin: string) =>
    post(machine ? `remote/${encodeURIComponent(machine)}/sunshine/pin` : "sunshine/pin", { pin }, true),
  jobsOf: (machine: string | null) => read<JobRecord[]>(queryPath("activity/jobs", machine), true),
  actionsOf: (machine: string | null) => read<ActivityEntry[]>(queryPath("activity", machine), true),
  jobHistory: read<JobRecord[]>("activity/jobs"),
  healthHistory: read<HealthObservation[]>("machines/history"),
  recentActivity: read<ActivityEntry[]>("activity"),
  /** A machine's specs and load: this one, or another by name, whichever dashboard is shown. */
  statsOf: (machine: string | null) => read<Stats>(queryPath("stats", machine), true),
  /** Asks a program on machine (null: this one) to quit, or with force ends it. */
  endProcess: (machine: string | null, pid: number, force: boolean) =>
    post(machine ? `remote/${encodeURIComponent(machine)}/processes/end` : "processes/end", { pid, force }, true),
  proxyLogins: read<LoginStatus>("proxy/logins"),
  t3: read<T3State>("t3", false, (data, previous) => {
    if (data.installed && data.problem && !data.linked && previous?.linked) throw new Error(data.problem);
  }),
  t3PhonesOf: (machine: string | null) => read<T3Phones>(queryPath("t3/phones", machine), true),
  t3Desktop: (machine: string) => call<{link: string}>("POST", "t3/desktop", { machine }, true),
  t3Sync: () => post("t3/sync"),
  t3Setup: () => post("t3/setup"),
  scan: () => call<Device[]>("GET", "machines/scan"),
  setHubOnly: (hub_only: boolean) => post("accounts/mode", { hub_only }),
  fix: (fix: string) => post("fix", { fix }),
  setup: (name: string, role: string) => post("setup", { name, role }),
  setupPreview: (name: string, role: string) => call<SetupPreview>("GET", "setup/preview?" + new URLSearchParams({ name, role }), undefined, true),
  setupProgress: () => call<SetupProgress>("GET", "setup", undefined, true),
  answerSetup: (password: string, cancel = false) => post("setup/answer", { password, cancel }),
  update: read<UpdateState>("update"),
  rolloutPlan: () => call<FleetPreview>("GET", "update/fleet", undefined, true),
  startUpdate: () => post("update"),
  setAutoUpdate: (machine: string | null, auto: boolean) => call<Done>("POST", queryPath("update/auto", machine), { auto }, true),
  restart: () => post("restart"),
  updateSystem: () => post("update/system"),
  setAccess: (device_access: boolean, terminals: boolean) => post("settings/access", { device_access, terminals }),
  t3Pair: (label: string) => call<{ code: string }>("POST", "t3/pair", { label }),

  ssh: read<SSHState>("ssh"),
  sshEnable: (on: boolean) => post("ssh/enable", { on }),
  sshSync: () => post("ssh/sync"),

  terminalOn: (machine: string | null) => {
    const path = (endpoint: string) => queryPath(endpoint, machine);
    return {
      start: (cols: number, rows: number) => call<{ id: string }>("POST", path("term"), { cols, rows }, true),
      output: async (id: string, signal: AbortSignal): Promise<ReadableStream<Uint8Array>> => {
        const endpoint = path(`term/${encodeURIComponent(id)}/out`);
        const res = await fetch(`/api/${endpoint}`, { credentials: "same-origin", signal });
        if (!res.ok || !res.body) throw await responseError(endpoint, res);
        return res.body;
      },
      input: (id: string, data: string) => call<void>("POST", path(`term/${encodeURIComponent(id)}/in`), { data }, true),
      resize: (id: string, cols: number, rows: number) => call<void>("POST", path(`term/${encodeURIComponent(id)}/size`), { cols, rows }, true),
      close: (id: string) => call<void>("POST", path(`term/${encodeURIComponent(id)}/close`), {}, true),
    };
  },
};
