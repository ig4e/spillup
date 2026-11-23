import type { Hold, PeerState } from "./types";

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

export function relative(iso?: string): string {
  if (!iso) return "—";
  const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  if (abs < 45) return "just now";
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(seconds / 3600), "hour");
  return rtf.format(Math.round(seconds / 86400), "day");
}

export function secondsAgo(s: number): string {
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  return `${Math.round(s / 3600)}h ago`;
}

export const holdLabel: Record<Hold, string> = {
  hub: "Main",
  standby: "Worker + Standby",
  never: "Worker",
};

export const holdHelp: Record<Hold, string> = {
  hub: "Holds the accounts whenever it is on.",
  standby: "Takes the accounts while the main machine is off, and hands them back.",
  never: "Uses the accounts through the machine holding them.",
};

export function stateColor(state?: PeerState): "success" | "warning" | "danger" | "default" | "accent" {
  switch (state) {
    case "leading":
      return "accent";
    case "synced":
      return "success";
    case "starting":
    case "standing":
      return "warning";
    case "silent":
      return "danger";
    default:
      return "default";
  }
}

/** "claude-work@example.com.json" → { provider: "claude", label: "work@example.com" } */
export function describeLogin(name: string, type?: string) {
  const base = name.replace(/\.json$/, "");
  const provider = type || base.split("-")[0] || "login";
  let label = base.startsWith(provider + "-") ? base.slice(provider.length + 1) : base;
  label = label.replace(/^[0-9a-f]{8}-/, "");
  return { provider, label };
}

export function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
