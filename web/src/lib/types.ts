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
  waiting?: string;
  in_flight?: number;
  accounts?: Account[];
  peers?: Peer[];
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
  };
  router_url: string;
  dashboard_url?: string;
}

export interface Machine {
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
