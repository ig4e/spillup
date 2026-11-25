import type { ActivityEntry, JobRecord } from "./types";

export type Outcome = "Accepted" | "Completed" | "Failed" | "Running";
interface Area { name: string; doctor?: string }
const areas: Record<string, Area> = {
  accounts: { name: "Accounts", doctor: "accounts" },
  own: { name: "Accounts", doctor: "cliproxyapi" },
  t3: { name: "T3 Code", doctor: "t3-code" },
  skills: { name: "Skills & MCP", doctor: "agents" },
  mcp: { name: "Skills & MCP", doctor: "agents" },
  update: { name: "Updates", doctor: "spinup" },
  restart: { name: "Updates", doctor: "spinup" },
  remote: { name: "Remote desktop", doctor: "network" },
  sunshine: { name: "Remote desktop", doctor: "network" },
  ssh: { name: "SSH", doctor: "network" },
  machines: { name: "Machines", doctor: "accounts" },
  setup: { name: "Setup" },
  term: { name: "Terminal", doctor: "network" },
  processes: { name: "Processes" },
};
const dashboard: Area = { name: "Dashboard" };
const jobNames: Record<string, string> = { setup: "Machine setup", update: "System update", remote: "Remote desktop setup", ssh: "SSH setup" };
const labels: Record<string, string> = {
  "accounts/handoff": "Move accounts", "accounts/mode": "Change account storage", "accounts/takeover": "Take over accounts",
  "accounts/restart": "Restart proxy", "machines/main": "Change hub", "processes/end": "End process",
  "settings/access": "Change dashboard access", "t3/link": "Link T3 environment", "t3/pair": "Create T3 pairing",
  "t3/sync": "Sync T3 machines", "update/system": "System update", "remote/setup": "Remote desktop setup",
  "ssh/enable": "Change SSH access", setup: "Machine setup", term: "Open terminal",
};

export interface ActivityItem {
  key: string; at: string; title: string; area: Area; outcome: Outcome;
  action?: ActivityEntry; job?: JobRecord;
}
function outcome(action?: ActivityEntry, job?: JobRecord): Outcome {
  if (action && (action.status < 200 || action.status >= 300)) return "Failed";
  if (!job) return "Accepted";
  if (job.state === "completed") return "Completed";
  return job.state === "running" ? "Running" : "Failed";
}

export function activityItems(actions: ActivityEntry[], jobs: JobRecord[]): ActivityItem[] {
  const byAction = new Map(jobs.filter(job => job.action_id).map(job => [job.action_id!, job]));
  const linked = new Set<JobRecord>();
  const items = actions.map((action): ActivityItem => {
    const job = action.id ? byAction.get(action.id) : undefined;
    if (job) linked.add(job);
    return { key: action.id ?? [action.at, action.origin, action.action].join("/"), at: action.at, action, job,
      title: labels[action.action] ?? action.action.replaceAll("/", " · "), area: areas[action.action.split("/")[0]] ?? dashboard, outcome: outcome(action, job) };
  });
  for (const job of jobs) {
    if (!linked.has(job)) items.push({ key: "job/" + job.started, at: job.started, job, title: jobNames[job.task] ?? job.task,
      area: areas[job.task] ?? dashboard, outcome: outcome(undefined, job) });
  }
  return items.sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
}
