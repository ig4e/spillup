import type { ActivityEntry, Doctor, HealthObservation, JobRecord, Overview } from "./types";

const checkIDs = new Set([
  "tools.install", "network.connection", "network.login-expiry", "network.certificates", "network.dns", "network.path",
  "accounts.setup", "accounts.service", "accounts.holder", "accounts.startup", "accounts.proxy-version", "accounts.logins",
  "agents.skills", "agents.config", "packages.plan", "spinup.version", "t3.install", "t3.startup", "t3.network",
  "t3.restart", "t3.quota-key", "t3.linking", "t3.environments", "proxy.port", "proxy.service", "proxy.key", "proxy.own",
]);
const sections = new Map([
  ["Tools", "tools"], ["Network", "network"], ["Accounts", "accounts"], ["Agents", "agents"],
  ["Packages", "packages"], ["CLIProxyAPI", "cliproxyapi"], ["T3 Code", "t3-code"], ["spinup", "spinup"],
]);
export const doctorAnchor = (title: string) => sections.has(title) ? `doctor-${sections.get(title)}` : undefined;

const healthChecks = ["Tailscale", "Spillup", "Account sync", "T3 linking", "T3 link from here"];
const healthStates = ["Online", "Offline", "Login expired", "Status unavailable", "Disconnected", "Responding",
  "No response", "Worker", "Holds the accounts", "Starting proxy", "Copies disabled", "In sync", "Awaiting fresh copy",
  "CLI absent", "Linking off", "Network access off", "Restart needed", "Environment list unavailable", "Tailnet address unavailable",
  "Ready", "Mobile client", "Paired", "Pairing failed"];
const components = ["setup", "update", "restart", "accounts", "machines", "own", "skills", "mcp", "t3", "remote",
  "sunshine", "processes", "term", "settings", "ssh", "fix"];

function allowed(value: string, choices: readonly string[]) {
  return choices.includes(value) ? value : "unknown";
}

function version(value?: string) {
  return value && /^(?:dev|v?\d{1,5}\.\d{1,5}\.\d{1,5})$/.test(value) ? value : "unknown";
}

function sinceMinutes(timestamp: string, now: Date) {
  const minutes = Math.floor((now.getTime() - Date.parse(timestamp)) / 60_000);
  return Number.isFinite(minutes) && minutes >= 0 ? minutes : null;
}

export interface SnapshotInput {
  now: Date;
  overview?: Overview;
  doctor?: Doctor;
  staleChecks: boolean;
  actions: ActivityEntry[] | null;
  jobs: JobRecord[] | null;
  health: HealthObservation[] | null;
}

// Project fields explicitly. Check labels, fix commands and errors contain user data.
export function supportSnapshot({ now, overview, doctor, staleChecks, actions, jobs, health }: SnapshotInput) {
  const checks = doctor?.sections.slice(0, 16).flatMap(section => {
    const counts = new Map<string, { id: string; outcome: string; count: number }>();
    for (const check of section.checks) {
      const id = check.id && checkIDs.has(check.id) ? check.id : (sections.get(section.title) ?? "unknown");
      const outcome = allowed(check.level, ["ok", "warn", "fail"]);
      const key = id + "/" + outcome;
      const entry = counts.get(key);
      if (entry) entry.count++;
      else counts.set(key, { id, outcome, count: 1 });
    }
    return [...counts.values()];
  }) ?? null;

  const peers = new Map<string, string>();
  const alias = (name: string) => {
    if (!name) return "unknown";
    if (name === overview?.name) return "this-machine";
    if (!peers.has(name)) peers.set(name, `peer-${peers.size + 1}`);
    return peers.get(name)!;
  };
  return {
    format: 1,
    created_at: now.toISOString(),
    machine: overview ? {
      alias: "this-machine",
      system: allowed(overview.os, ["linux", "windows", "darwin", "freebsd"]),
      spillup_version: version(overview.version),
      role: allowed(overview.hold, ["hub", "standby", "never"]),
      accounts_service: overview.service ? {
        version: version(overview.service.version), serving: overview.service.leading === true, proxy_running: overview.service.proxy_running === true,
      } : null,
    } : null,
    checks_from_last_successful_refresh: !!doctor && staleChecks,
    checks,
    recent_actions: actions?.slice(0, 20).map(a => ({
      component: allowed(a.action.split("/", 1)[0], components),
      status: Number.isInteger(a.status) && a.status >= 100 && a.status <= 599 ? a.status : null,
      minutes_ago: sinceMinutes(a.at, now),
    })) ?? null,
    recent_jobs: jobs?.slice(0, 10).map(j => ({
      task: allowed(j.task, ["setup", "update", "remote", "ssh"]),
      outcome: allowed(j.state, ["running", "completed", "failed", "timed out", "interrupted"]),
      minutes_ago: sinceMinutes(j.updated, now),
    })) ?? null,
    health_changes: health?.slice(0, 32).map(h => ({
      machine: alias(h.machine), check: allowed(h.check, healthChecks), state: allowed(h.state, healthStates),
      minutes_ago: sinceMinutes(h.since, now),
    })) ?? null,
  };
}
