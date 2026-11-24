// A pretend fleet for `npm run dev:mock`, so the dashboard can be designed without a tailnet.
import type { Doctor, Hold, Keys, Machines, Overview, Skills } from "./types";

const ago = (s: number) => new Date(Date.now() - s * 1000).toISOString();
const wait = (ms = 350) => new Promise((r) => setTimeout(r, ms));

const fleet = {
  self: "laptop",
  leader: "desktop",
  holds: { desktop: "hub", laptop: "standby" } as Record<string, Hold>,
};

let hubOnly = false;
const own = { providers: ["claude"] as string[], logins: [{ name: "claude-laptop@example.com.json", type: "claude", refreshed: ago(600) }] };

const skills: Skills = {
  library: "~/.spinup",
  token_estimate: 1840,
  shared: [
    { name: "frontend-design", source: "anthropics/skills", mode: "auto", installed: true, description_chars: 1420, agents: ["claude", "codex"] },
    { name: "webapp-testing", source: "anthropics/skills", mode: "manual", installed: true, description_chars: 980, agents: ["claude", "codex"] },
    { name: "go-conventions", source: "yours", mode: "auto", installed: true, description_chars: 2210, agents: ["claude", "codex"] },
    { name: "release-notes", source: "yours", mode: "manual", installed: false, description_chars: 640, agents: [] },
    { name: "systematic-debugging", source: "obra/superpowers", mode: "auto", installed: true, description_chars: 1730, agents: ["claude"] },
    { name: "pdf", source: "anthropics/skills", mode: "manual", installed: true, description_chars: 1180, agents: ["claude", "codex"] },
  ],
  only_here: [
    { name: "scratch-notes", agent_own: false, agents: ["claude"] },
    { name: "codex-review", agent_own: true, agents: ["codex"], path: "~/.codex/skills/codex-review" },
  ],
  removed: [{ name: "old-deploy", was: "old-deploy", removed: ago(86400 * 3) }],
};

function overview(): Overview {
  const hold = fleet.holds[fleet.self] ?? "never";
  return {
    installed: true,
    name: fleet.self,
    hold,
    os: "linux",
    version: "v0.2.0-nightly.31",
    can_manage: hold !== "never",
    service: {
      name: fleet.self,
      hold,
      version: "v0.2.0-nightly.31",
      leading: fleet.leader === fleet.self,
      epoch: 14,
      leader: fleet.leader,
      leader_addr: `${fleet.leader}:8317`,
      synced: { epoch: 14, seconds_ago: 2 },
      proxy_running: fleet.leader === fleet.self,
      in_flight: fleet.leader === fleet.self ? 2 : 0,
      hub_only: hubOnly,
      accounts: [
        { name: "claude-work@example.com.json", type: "claude", refreshed: ago(1800) },
        { name: "codex-pro@example.com.json", type: "codex", refreshed: ago(240) },
        { name: "antigravity-personal@example.com.json", type: "antigravity", refreshed: ago(7200) },
        { name: "antigravity-work@example.com.json", type: "antigravity", refreshed: ago(5400) },
      ],
      peers: Object.entries(fleet.holds)
        .filter(([n]) => n !== fleet.self)
        .map(([name, h]) => ({
          name,
          hold: h,
          state: name === fleet.leader ? "leading" : "synced",
          epoch: 14,
          offline_for: 0,
          may_have_led: false,
        })),
    },
    settings: { port: 8317, proxy_port: 8327, failover_after_seconds: 180, auto_failback: true, library: "~/.spinup" },
    router_url: "http://localhost:8317",
    dashboard_url: "http://localhost:8317/management.html",
  };
}

function machines(): Machines {
  const base = [
    { name: "laptop", os: "linux", ip: "100.101.12.4", online: true },
    { name: "desktop", os: "windows", ip: "100.88.30.17", online: true },
    { name: "macbook", os: "macOS", ip: "100.70.2.9", online: false, last_seen: ago(3600 * 5) },
    { name: "phone", os: "iOS", ip: "100.99.1.33", online: true },
  ];
  return {
    machines: base.map((m) => ({
      ...m,
      dns_name: `${m.name}.tail1234.ts.net`,
      is_self: m.name === fleet.self,
      hold: fleet.holds[m.name],
      state: fleet.holds[m.name] ? (m.name === fleet.leader ? "leading" : "synced") : undefined,
      leading: m.name === fleet.leader,
    })),
  };
}

const doctor: Doctor = {
  failures: 1,
  sections: [
    { title: "Tools", checks: [{ level: "ok", label: "all 14 tools installed" }] },
    {
      title: "Network",
      checks: [
        { level: "ok", label: "Tailscale is up as laptop (100.101.12.4)" },
        { level: "ok", label: "desktop: direct connection" },
        { level: "warn", label: "macbook: offline for 5 hours", fix: "turn it on, or ignore it if it's away" },
      ],
    },
    {
      title: "Accounts",
      checks: [
        { level: "ok", label: "the service runs here as standby, synced 2s ago" },
        { level: "ok", label: "4 logins reach localhost:8317 through desktop" },
      ],
    },
    {
      title: "Agents",
      checks: [{ level: "fail", label: "release-notes isn't installed here", fix: "spinup skills sync" }],
    },
    { title: "Update", checks: [{ level: "ok", label: "spinup v0.2.0-nightly.31 is the newest" }] },
  ],
};

export async function mock(method: string, fullPath: string, body: any): Promise<unknown> {
  await wait();
  const viewing = fullPath.match(/^remote\/([^/]+)\//)?.[1];
  const path = viewing ? fullPath.slice(fullPath.indexOf("/", 7) + 1) : fullPath;
  if (viewing && method === "GET" && path === "overview") {
    const o = overview();
    const name = decodeURIComponent(viewing);
    const hold = fleet.holds[name] ?? "never";
    return { ...o, name, hold, can_manage: hold !== "never", os: "windows", service: { ...o.service!, name, hold, leading: fleet.leader === name } };
  }
  if (method === "GET" && path.startsWith("own/signin?")) return { status: "ok" };
  if (method === "GET" && fullPath.endsWith("stats")) return mockStats(viewing ?? "laptop");
  if (method === "GET") {
    switch (path) {
      case "overview":
        return overview();
      case "machines":
        return machines();
      case "doctor":
        await wait(900);
        return doctor;
      case "skills":
        return structuredClone(skills);
      case "own":
        return {
          providers: own.providers,
          available: [
            { id: "claude", name: "Claude", sign_in: true },
            { id: "codex", name: "ChatGPT & Codex", sign_in: true },
            { id: "antigravity", name: "Google Antigravity", sign_in: true },
            { id: "kimi", name: "Kimi", sign_in: true },
            { id: "xai", name: "Grok (xAI)", sign_in: true },
            { id: "devin", name: "Devin", sign_in: true },
            { id: "meta", name: "Meta AI", sign_in: true },
          ],
          running: own.providers.length > 0,
          logins: own.logins,
        };
      case "apps":
        return [
          { id: "claude", name: "Claude Code", installed: true },
          { id: "codex", name: "Codex", installed: true },
          { id: "t3", name: "T3 Code", installed: true },
          { id: "gemini", name: "Gemini CLI", installed: false },
          { id: "opencode", name: "OpenCode", installed: true },
          { id: "ccp", name: "ccp", installed: true },
        ];
      case "proxy/logins":
        return {
          logins: [
            { name: "claude-work@example.com.json", status: "active", status_message: "", unavailable: false, disabled: false },
            { name: "codex-pro@example.com.json", status: "active", status_message: "", unavailable: false, disabled: false },
            { name: "antigravity-personal@example.com.json", status: "error", status_message: "429 quota exhausted, resets in 2h", unavailable: true, disabled: false },
            { name: "antigravity-work@example.com.json", status: "active", status_message: "", unavailable: false, disabled: false },
          ],
        };
      case "remote":
        return {
          sunshine_installed: true, sunshine_running: true, moonlight_installed: true, web_installed: true, web_running: true,
          hosts: [{ id: 1, name: "DESKTOP-7Q2", machine: "desktop", paired: true, state: "Free" }, { id: 2, name: "nas", machine: "nas", paired: false, state: "Free" }],
        };
      case "machines/scan":
        await wait(900);
        return [
          { name: "desktop", ip: "100.88.30.17", os: "windows", online: true, yours: true, spillup: true, hold: fleet.holds.desktop },
          { name: "macbook", ip: "100.70.2.9", os: "macOS", online: false, yours: true, spillup: false },
          { name: "phone", ip: "100.99.1.33", os: "iOS", online: true, yours: true, spillup: false },
          { name: "nas", ip: "100.80.4.2", os: "linux", online: true, yours: true, spillup: true, hold: "never" },
        ];
      case "setup":
        return { running: false, done: false, log: [] };
      case "t3":
        return { installed: true, app: true, port: 3773, linked: '[{"url":"http://100.88.30.17:3773","label":"desktop"}]' };
      case "update":
        return { current: "v0.3.6", latest: "v0.3.7", auto: true };
      case "keys":
        return { api_key: "sk-spinup-3f9c1e7a52b84d06a1c2e9f0", management_password: "m-81b2c4d6e8f0a1b3" } satisfies Keys;
    }
  }
  if (path.startsWith("own/signin?")) return { status: "ok" };
  switch (path) {
    case "own":
      own.providers = body.providers;
      return { message: body.providers.length ? `This machine now signs in to ${body.providers.join(", ")} by itself.` : "Every provider uses the shared accounts again." };
    case "remote/sync":
      await wait(1200);
      return { message: "1 machine(s) newly paired for remote desktop." };
    case "remote/setup":
      return { message: "Setting up remote desktop…" };
    case "sunshine/pin":
      return { message: "Paired." };
    case "accounts/mode":
      hubOnly = body.hub_only;
      return { message: body.hub_only ? "Only the main machine keeps the logins now." : "Standbys keep copies again." };
    case "fix":
      await wait(1000);
      return { message: "12 skills for claude, codex." };
    case "t3/setup":
      await wait(1200);
      return { message: "T3 Code starts at boot now, with linking on." };
    case "t3/sync":
      await wait(1200);
      return { message: "T3 Code on 3 machine(s); 1 new link(s)." };
    case "update":
      return { message: "Updating. spillup restarts in a minute; this page reconnects by itself." };
    case "restart":
      return { message: "Restarting spillup. This page reconnects in a few seconds." };
    case "update/auto":
      return { message: body.auto ? "spillup now updates itself when a new version is out." : "Automatic updates are off." };
    case "own/signin":
      own.logins.push({ name: `${body.provider}-laptop2@example.com.json`, type: body.provider, refreshed: new Date().toISOString() });
      return { url: "about:blank", state: "mock" };
    case "own/signout":
      own.logins = own.logins.filter((l) => l.name !== body.name);
      return { message: "Signed out on this machine." };
    case "machines/main": {
      for (const n of Object.keys(fleet.holds)) fleet.holds[n] = "standby";
      fleet.holds[body.machine] = "hub";
      setTimeout(() => (fleet.leader = body.machine), 2500);
      return { message: `${body.machine} is the hub now. The accounts move to it between requests.` };
    }
    case "accounts/handoff":
      await wait(1500);
      fleet.leader = body.to;
      return { message: `${body.to} holds the accounts.` };
    case "accounts/restart":
      await wait(1200);
      return { message: "CLIProxyAPI restarted; the accounts stayed here." };
    case "accounts/takeover":
      throw new Error("another machine holds the accounts right now (desktop)");
    case "skills/mode":
      for (const s of skills.shared) if (body.names.includes(s.name)) s.mode = body.manual ? "manual" : "auto";
      return { message: `${body.names.join(", ")} ${body.manual ? "manual" : "auto"} now.` };
    case "skills/remove":
      skills.shared = skills.shared.filter((s) => {
        if (!body.names.includes(s.name)) return true;
        skills.removed.unshift({ name: s.name, was: s.name, removed: new Date().toISOString() });
        return false;
      });
      return { message: `Removed ${body.names.join(", ")} from every machine.` };
    case "skills/restore": {
      skills.removed = skills.removed.filter((r) => r.name !== body.name);
      skills.shared.push({ name: body.name, source: "yours", mode: "auto", installed: true, description_chars: 800, agents: ["claude", "codex"] });
      return { message: `${body.name} is back in your library, on every machine.` };
    }
    case "skills/adopt":
      skills.only_here = skills.only_here.filter((s) => !body.names.includes(s.name));
      for (const n of body.names) skills.shared.push({ name: n, source: "yours", mode: "auto", installed: true, description_chars: 500, agents: ["claude", "codex"] });
      return { message: `${body.names.join(", ")} in your library now.` };
    case "skills/add":
      await wait(1500);
      for (const n of body.names) skills.shared.push({ name: n, source: body.repo, mode: body.manual ? "manual" : "auto", installed: true, description_chars: 900, agents: ["claude", "codex"] });
      return { message: `Added ${body.names.join(", ")} from ${body.repo}.` };
    case "skills/sync":
      await wait(1500);
      for (const s of skills.shared) s.installed = true;
      return { message: `${skills.shared.length} skills installed for claude, codex.` };
  }
  throw new Error(`mock: no ${method} ${path}`);
}

const statSeeds: Record<string, { cpu: number; cores: number; mem: number; model: string; os: string }> = {
  laptop: { cpu: 18, cores: 16, mem: 32, model: "AMD Ryzen 7 7840U", os: "linux" },
  desktop: { cpu: 42, cores: 24, mem: 64, model: "AMD Ryzen 9 7950X3D", os: "windows" },
  nas: { cpu: 6, cores: 4, mem: 16, model: "Intel N100", os: "linux" },
};
function mockStats(name: string) {
  const seed = statSeeds[name] ?? statSeeds.laptop;
  const gb = 1024 ** 3;
  const now = Date.now();
  const history = Array.from({ length: 60 }, (_, i) => {
    const wave = Math.sin((now / 4000 + i) / 4);
    return {
      at: new Date(now - (60 - i) * 2000).toISOString(),
      cpu: Math.max(2, seed.cpu + wave * 12 + Math.random() * 6),
      mem_used: seed.mem * gb * (0.48 + wave * 0.03),
      disk_used: 610 * gb,
      load1: seed.cores * (seed.cpu / 100) * (1 + wave * 0.2),
      rx_rate: 1.2e6 * (1.5 + wave) + Math.random() * 3e5,
      tx_rate: 3.4e5 * (1.4 + wave) + Math.random() * 1e5,
      rx_total: 48 * gb,
      tx_total: 9 * gb,
      uptime: 3 * 86400 + 4 * 3600,
    };
  });
  const mb = 1024 ** 2;
  const names = seed.os === "windows"
    ? ["t3code.exe", "spinup.exe", "cli-proxy-api.exe", "chrome.exe", "Code.exe", "node.exe", "tailscaled.exe", "explorer.exe", "dwm.exe", "claude.exe"]
    : ["t3code", "spinup", "cli-proxy-api", "chromium", "node", "claude", "codex", "tailscaled", "Hyprland", "go"];
  const processes = names.map((n, i) => ({
    pid: 1200 + i * 37,
    name: n,
    cpu: Math.max(0, (seed.cpu / (i + 1.4)) * (1 + Math.sin(now / 3000 + i) * 0.4)),
    mem: (900 / (i + 1)) * mb + i * 40 * mb,
  }));
  return {
    processes,
    process_count: 312,
    specs: { hostname: name, os: seed.os, arch: "amd64", cpu_model: seed.model, cores: seed.cores, mem_total: seed.mem * gb, disk_total: 1000 * gb },
    now: history[history.length - 1],
    history,
  };
}
