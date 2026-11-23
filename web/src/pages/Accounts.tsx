import { Bot, Code2, ExternalLink, KeyRound, LifeBuoy, Orbit, RotateCw, Server, Terminal } from "lucide-react";
import { useOverview } from "../App";
import { useConfirm } from "../components/Confirm";
import { Dot, Group, IconTile, LargeTitle, Mono, Row, type Tint } from "../components/ui";
import { api } from "../lib/api";
import { capitalize, describeLogin, relative } from "../lib/format";
import { useAction } from "../lib/hooks";

const provider: Record<string, { icon: typeof Bot; color: Tint }> = {
  claude: { icon: Bot, color: "orange" },
  codex: { icon: Code2, color: "graphite" },
  antigravity: { icon: Orbit, color: "indigo" },
  gemini: { icon: Orbit, color: "blue" },
};

export function AccountsPage() {
  const overview = useOverview();
  const { run } = useAction();
  const { ask, dialog } = useConfirm();
  const data = overview.data;
  const s = data?.service;
  const accounts = s?.accounts ?? [];
  const holds = data?.hold === "hub" || data?.hold === "standby";
  const leader = s?.leading ? data?.name : s?.leader || s?.leader_addr?.split(":")[0];

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
      body: (
        <>
          Only do this if the machine that held the accounts is gone for good. If it comes back while this machine holds them, both
          refresh the same logins and the accounts get logged out. spillup already fails over by itself when a machine goes offline.
        </>
      ),
      run: () => run("takeover", api.takeover, overview.reload),
    });

  return (
    <>
      <LargeTitle title="Accounts" subtitle="Your Claude, Codex and other logins: one machine holds them, every machine uses them." />

      <Group footer={<>Run <Mono>ccp</Mono> instead of <Mono>claude</Mono> to use the shared accounts in Claude Code. In T3 Code, add a provider pointed at the address above.</>}>
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

      {holds ? (
        <Group title="CLIProxyAPI on This Machine">
          <Row
            icon={<IconTile icon={Terminal} color="graphite" />}
            title="Status"
            value={s?.proxy_running ? (s.in_flight ? `Running · ${s.in_flight} requests` : "Running") : "Standing by"}
          />
          {data?.can_manage && s?.leading ? <Row icon={<IconTile icon={RotateCw} color="blue" />} title="Restart CLIProxyAPI…" tone="accent" onPress={restart} /> : null}
          {data?.dashboard_url && s?.leading ? (
            <Row
              icon={<IconTile icon={ExternalLink} color="teal" />}
              title="Add or Remove Logins"
              subtitle="Opens the CLIProxyAPI dashboard"
              tone="accent"
              onPress={() => window.open(data.dashboard_url, "_blank", "noopener")}
            />
          ) : null}
        </Group>
      ) : null}

      {holds ? (
        <Group title="Logins" footer={accounts.length ? `${accounts.length} logins, shared with every machine.` : "Add logins in the CLIProxyAPI dashboard on the machine holding the accounts."}>
          {accounts.length ? (
            accounts.map((a) => {
              const { provider: p, label } = describeLogin(a.name, a.type);
              const look = provider[p] ?? { icon: KeyRound, color: "gray" as Tint };
              return (
                <Row
                  key={a.name}
                  icon={<IconTile icon={look.icon} color={look.color} />}
                  title={label}
                  subtitle={capitalize(p)}
                  value={<span className="text-xs">refreshed {relative(a.refreshed)}</span>}
                />
              );
            })
          ) : (
            <Row title={<span className="text-muted">No logins yet</span>} />
          )}
        </Group>
      ) : (
        <Group title="Logins" footer={`This machine keeps no copy of the logins; it uses them through ${leader ?? "the main machine"}. Open spillup there to see them.`}>
          <Row icon={<IconTile icon={KeyRound} color="gray" />} title="Kept on the main machine and standbys" />
        </Group>
      )}

      {holds && data?.can_manage && !s?.leading ? (
        <Group footer="Use only if the machine that held the accounts was lost for good.">
          <Row icon={<IconTile icon={LifeBuoy} color="red" />} title="Take Over Accounts Here…" tone="danger" onPress={takeover} />
        </Group>
      ) : null}
      {dialog}
    </>
  );
}
