import { ExternalLink, KeyRound, LifeBuoy, RotateCw, Server, Terminal } from "lucide-react";
import { useOverview } from "../App";
import { useConfirm } from "../components/Confirm";
import { Dot, Group, IconTile, LargeTitle, Mono, Row } from "../components/ui";
import { api } from "../lib/api";
import { describeLogin, relative } from "../lib/format";
import { hintsFor } from "../lib/hints";
import { useAction, useQuery } from "../lib/hooks";
import { providerLook } from "../lib/providers";
import type { LoginState } from "../lib/types";

/** How CLIProxyAPI sees a login right now, in a few plain words. */
function loginHealth(state?: LoginState): { tone: "success" | "warning" | "danger" | "muted"; text: string } {
  if (!state) return { tone: "muted", text: "" };
  if (state.disabled) return { tone: "muted", text: "Off" };
  const msg = state.status_message.toLowerCase();
  if (msg.includes("invalid_grant") || msg.includes("401") || msg.includes("revoked")) return { tone: "danger", text: "Signed out" };
  if (state.unavailable || msg.includes("429") || msg.includes("quota")) return { tone: "warning", text: "Cooling down" };
  return { tone: "success", text: "Active" };
}

export function AccountsPage() {
  const overview = useOverview();
  const apps = useQuery(api.apps);
  const status = useQuery(api.proxyLogins, 15000);
  const { run } = useAction();
  const { ask, dialog } = useConfirm();
  const data = overview.data;
  const s = data?.service;
  const accounts = s?.accounts ?? [];
  const holds = data?.hold === "hub" || data?.hold === "standby";
  const leader = s?.leading ? data?.name : s?.leader || s?.leader_addr?.split(":")[0];
  const states = new Map((status.data?.logins ?? []).map((l) => [l.name, l]));

  const restart = () =>
    ask({
      title: "Restart CLIProxyAPI?",
      status: "accent",
      confirm: "Restart",
      body: "Running requests finish first, and the accounts stay on this machine. Use this after updating CLIProxyAPI.",
      run: () => run("restart", api.restartProxy, overview.reload),
    });

  const takeover = () =>
    ask({
      title: "Take the accounts here?",
      status: "danger",
      confirm: "Take Over",
      body: "Only do this if the machine that held the accounts is gone for good. If it comes back while this machine holds them, both refresh the same logins and the accounts get logged out. spillup already fails over by itself when a machine goes offline.",
      run: () => run("takeover", api.takeover, overview.reload),
    });

  return (
    <>
      <LargeTitle title="Accounts" subtitle="Your subscriptions: one machine holds them, every machine uses them." />

      <Group>
        <Row
          icon={<IconTile icon={KeyRound} color={leader ? "green" : "orange"} />}
          title="Held by"
          value={
            <span className="flex items-center gap-1.5 font-medium text-foreground">
              <Dot tone={leader ? "success" : "warning"} /> {leader ?? "No machine yet"}
              {s?.leading ? <span className="font-normal text-muted">(this machine)</span> : null}
            </span>
          }
        />
        <Row icon={<IconTile icon={Server} color="blue" />} title="Address on every machine" accessory={<Mono copy>{data?.router_url ?? "http://localhost:8317"}</Mono>} />
      </Group>

      <Group title="Use Them in Your Apps">
        {hintsFor(apps.data, data).map((h) => (
          <Row key={h.app} title={h.app} subtitle={<span className="whitespace-normal">{h.text}</span>} accessory={h.code ? <Mono copy>{h.code}</Mono> : undefined} />
        ))}
      </Group>

      {holds ? (
        <Group
          title="Logins"
          footer={
            status.data?.problem
              ? status.data.problem
              : accounts.length
                ? `${accounts.length} logins, shared with every machine. Status comes from CLIProxyAPI.`
                : "Add logins in CLIProxyAPI's dashboard."
          }
        >
          {accounts.length ? (
            accounts.map((a) => {
              const { provider, label } = describeLogin(a.name, a.type);
              const look = providerLook(provider);
              const health = loginHealth(states.get(a.name));
              return (
                <Row
                  key={a.name}
                  icon={<IconTile icon={look.icon} color={look.color} />}
                  title={label}
                  subtitle={`${look.name} · refreshed ${relative(a.refreshed)}`}
                  value={
                    health.text ? (
                      <span className="flex items-center gap-1.5 text-[13px]">
                        <Dot tone={health.tone} /> {health.text}
                      </span>
                    ) : undefined
                  }
                />
              );
            })
          ) : (
            <Row title={<span className="text-muted">No logins yet</span>} />
          )}
          {data?.can_manage ? <Row icon={<IconTile icon={ExternalLink} color="teal" />} title="Add or Remove Logins" subtitle="In CLIProxyAPI’s dashboard" to="/proxy" /> : null}
        </Group>
      ) : (
        <Group title="Logins" footer={`This machine keeps no copy of the logins; it uses them through ${leader ?? "the main machine"}.`}>
          <Row icon={<IconTile icon={KeyRound} color="gray" />} title="Kept on the main machine and standbys" />
        </Group>
      )}

      {holds ? (
        <Group title="CLIProxyAPI on This Machine">
          <Row
            icon={<IconTile icon={Terminal} color="graphite" />}
            title="Status"
            value={s?.proxy_running ? (s.in_flight ? `Running · ${s.in_flight} requests` : "Running") : "Standing by"}
          />
          {data?.can_manage && s?.leading ? <Row icon={<IconTile icon={RotateCw} color="blue" />} title="Restart CLIProxyAPI…" tone="accent" onPress={restart} /> : null}
        </Group>
      ) : null}

      {holds && data?.can_manage && !s?.leading ? (
        <Group footer="Use only if the machine that held the accounts was lost for good.">
          <Row icon={<IconTile icon={LifeBuoy} color="red" />} title="Take Over Accounts Here…" tone="danger" onPress={takeover} />
        </Group>
      ) : null}
      {dialog}
    </>
  );
}
