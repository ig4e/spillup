import type { Machine } from "./types";

export const mobilePeer = (machine: Machine) => /^(android|ios|ipados)$/i.test(machine.os ?? "");

export function agentHosts(json?: string): Set<string> {
  const hosts = new Set<string>();
  let rows: unknown;
  try { rows = JSON.parse(json || "[]"); } catch { return hosts; }
  if (!Array.isArray(rows)) return hosts;
  for (const link of rows) {
    if (!link || link.status === "expired") continue;
    for (const endpoint of [link.url, ...(Array.isArray(link.urls) ? link.urls : [])]) {
      if (typeof endpoint !== "string") continue;
      try {
        const host = new URL(endpoint).hostname;
        if (host) hosts.add(normalizeHost(host));
      } catch { /* A link can have multiple advertised endpoints. */ }
    }
  }
  return hosts;
}

const normalizeHost = (host: string) => host.toLowerCase().replace(/^\[|\]$|\.$/g, "");

export function hasAgentLink(machine: Machine, hosts: Set<string>) {
  return [machine.name, machine.ip, machine.ipv6, machine.dns_name].some(host => host && hosts.has(normalizeHost(host)));
}
