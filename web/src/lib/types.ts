// Mirrors internal/webui/wire.go and internal/api (snake_case on the wire).

export type Hold = "hub" | "standby" | "never";

export type PeerState = "offline" | "silent" | "standing" | "synced" | "starting" | "leading";

export interface Peer {
  name: string;
  hold: Hold;
  state: PeerState;
  epoch: number;
  offline_for: number; // nanoseconds
  may_have_led: boolean;
}

export interface Account {
  name: string;
  type?: string;
  refreshed: string;
}

export interface Report {
  name: string;
  hold: Hold;
  version: string;
  leading: boolean;
  starting?: boolean;
  epoch: number;
  leader: string;
  leader_addr?: string;
  synced?: { epoch: number; seconds_ago: number };
  proxy_running: boolean;
  managed_proxy_port?: number;
  waiting?: string;
  in_flight?: number;
  accounts?: Account[];
  peers?: Peer[];
  own?: string[];
  own_running?: boolean;
  hub_only?: boolean;
}

export interface Overview {
  installed: boolean;
  name: string;
  hold: Hold;
  os: string;
  version: string;
  can_manage: boolean;
  service?: Report;
  service_problem?: string;
  settings: {
    port: number;
    proxy_port: number;
    failover_after_seconds: number;
    auto_failback: boolean;
    library?: string;
    device_access?: boolean;
    terminals?: boolean;
    device_url?: string;
  };
  router_url: string;
  dashboard_url?: string;
}

export interface Machine {
  yours?: boolean;
  name: string;
  dns_name?: string;
  ip?: string;
  os?: string;
  online: boolean;
  last_seen?: string;
  key_expiry?: string;
  expired?: boolean;
  is_self?: boolean;
  hold?: Hold;
  state?: PeerState;
  leading?: boolean;
}

export interface Machines {
  machines: Machine[];
  problem?: string;
}

export type Level = "ok" | "warn" | "fail";

export interface Doctor {
  sections: { title: string; checks: { level: Level; label: string; fix?: string }[] }[];
  failures: number;
}

export interface SharedSkill {
  name: string;
  source: string;
  mode: "auto" | "manual";
  installed: boolean;
  description_chars: number;
  agents: string[];
}

export interface LocalSkill {
  name: string;
  agent_own: boolean;
  agents: string[];
  path?: string;
}

export interface RemovedSkill {
  name: string;
  was: string;
  removed: string;
}

export interface Skills {
  library: string;
  token_estimate: number;
  shared: SharedSkill[];
  only_here: LocalSkill[];
  removed: RemovedSkill[];
  clashes?: { name: string; folders: string[] }[];
}

export interface Keys {
  api_key: string;
  management_password?: string;
}

export interface Done {
  message: string;
}

export interface ProviderInfo {
  id: string;
  name: string;
  sign_in: boolean;
}

export interface OwnState {
  providers: string[];
  available: ProviderInfo[];
  running: boolean;
  logins: Account[];
}

export interface App {
  id: string;
  name: string;
  installed: boolean;
}

export interface LoginState {
  name: string;
  status: string;
  status_message: string;
  unavailable: boolean;
  disabled: boolean;
}

export interface LoginStatus {
  logins: LoginState[];
  problem?: string;
}

export interface T3State {
  installed: boolean;
  app: boolean;
  network_access?: boolean;
  awaits_restart?: boolean;
  address?: string;
  port?: number;
  environment_id?: string;
  linked?: string;
  problem?: string;
}

export interface UpdateState {
  current: string;
  latest?: string;
  auto: boolean;
  system?: string[];
  system_managed?: boolean;
}

export interface SignIn {
  url: string;
  state: string;
}

export interface SignInStatus {
  status: "ok" | "wait" | "error";
  error?: string;
}

export interface Device {
  name: string;
  ip: string;
  os?: string;
  online: boolean;
  yours: boolean;
  spillup: boolean;
  hold?: Hold;
}

export interface HealthObservation {
  machine: string;
  check: string;
  state: string;
  since: string;
  last: string;
}

export interface JobRecord {
  started: string;
  updated: string;
  task: string;
  state: "running" | "failed" | "timed out" | "completed" | "interrupted";
  step?: number;
  total?: number;
}

export interface ActivityEntry {
  at: string;
  action: string;
  origin: string;
  status: number;
  elapsed_ms: number;
}

export interface SetupProgress {
  task?: string;
  phase?: string;
  step?: number;
  total?: number;
  elapsed_seconds?: number;
  quiet_seconds?: number;
  running: boolean;
  done: boolean;
  failed?: string;
  log: string[];
  prompt?: string;
}

export interface Specs {
  hostname: string;
  os: string;
  arch: string;
  cpu_model?: string;
  cores: number;
  mem_total: number;
  disk_total?: number;
}

export interface Sample {
  at: string;
  cpu: number;
  mem_used: number;
  disk_used?: number;
  load1?: number;
  rx_rate: number;
  tx_rate: number;
  rx_total: number;
  tx_total: number;
  uptime: number;
}

export interface Process {
  pid: number;
  name: string;
  cpu: number;
  mem: number;
}

export interface Stats {
  specs: Specs;
  now: Sample;
  history: Sample[];
  processes: Process[];
  process_count: number;
}

export interface RemoteHost {
  id: number;
  name: string;
  machine?: string;
  paired: boolean;
  state?: string;
}

export interface RemoteState {
  sunshine_installed: boolean;
  sunshine_running: boolean;
  moonlight_installed: boolean;
  web_installed: boolean;
  web_running: boolean;
  problem?: string;
  hosts: RemoteHost[];
}

export interface SSHState {
  server: "tailscale" | "openssh";
  on: boolean;
  user: string;
  address?: string;
  key?: string;
  hosts: string[];
  problem?: string;
}

export interface SetupPreview {
  name: string;
  role: Hold;
  changes: { action: string; target: string; detail: string }[];
  deferred: string;
}

export interface QuotaSummary {
  checked_at: string;
  accounts: {
    provider: string;
    state: "ready" | "sign-in" | "limited" | "disabled" | "unavailable" | "unknown";
    observed_at?: string;
    usage_status?: "refused" | "rate-limited" | "unavailable";
    windows?: { label: string; used_percent: number; resets_at?: string }[];
  }[];
}

export interface FleetPreview {
  output: string;
  problem?: string;
}
