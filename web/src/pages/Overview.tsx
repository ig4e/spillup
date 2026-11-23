import { Alert, Button } from "@heroui/react";
import { Crown, KeyRound, Network, RefreshCw, Sparkles, Terminal, Users } from "lucide-react";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, Widget, holdName, osName, osTile } from "../components/ui";
import { api } from "../lib/api";
import { relative, secondsAgo } from "../lib/format";
import { useQuery } from "../lib/hooks";

export function OverviewPage() {
  const { data, loading } = useOverview();
  const machines = useQuery(api.machines, 10000);
  const skills = useQuery(api.skills);

  if (loading && !data) {
    return (
      <>
        <LargeTitle title="Overview" />
        <GroupSkeleton rows={2} />
        <GroupSkeleton rows={4} />
      </>
    );
  }
  if (!data) return null;
  if (!data.installed) return <NotSetUp />;

  const s = data.service;
  const leader = s?.leading ? data.name : s?.leader || s?.leader_addr?.split(":")[0] || "";
  const all = machines.data?.machines ?? [];
  const online = all.filter((m) => m.online).length;
  const main = all.find((m) => m.hold === "hub");

  return (
    <>
      <LargeTitle title="Overview" subtitle="Everything your machines share." />

      {data.service_problem ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>The accounts service isn't answering</Alert.Title>
            <Alert.Description>{data.service_problem}. The Doctor page shows why.</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
      {s?.waiting ? (
        <Alert status="warning" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>Waiting</Alert.Title>
            <Alert.Description>{s.waiting}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}

      <div className="mb-7 flex flex-col items-center rounded-2xl bg-surface px-6 py-9 text-center shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
        <IconTile icon={KeyRound} color={leader ? "green" : "orange"} size="lg" />
        <div className="mt-4 text-[15px] text-muted">Your accounts are on</div>
        <div className="mt-0.5 text-[34px] leading-tight font-bold tracking-[-0.025em]">{leader || "no machine yet"}</div>
        <div className="mt-2 flex items-center gap-1.5 text-[13px] text-muted">
          <Dot tone={leader ? "success" : "warning"} />
          {leader ? (s?.in_flight ? `Serving ${s.in_flight} request${s.in_flight === 1 ? "" : "s"}` : "Ready") : "Waiting for the main machine"}
        </div>
        <div className="mt-4 text-[13px] text-muted">
          Every machine uses them at <Mono copy>{data.router_url}</Mono>
        </div>
      </div>

      <div className="mb-7 grid grid-cols-2 gap-3 md:grid-cols-4">
        <Widget label="Online" icon={<Network className="size-3.5" />} value={machines.data ? `${online} of ${all.length}` : "—"} detail="machines" />
        <Widget label="Main" icon={<Crown className="size-3.5" />} value={main?.name ?? "—"} detail={main ? (main.online ? "online" : "offline") : "not chosen"} />
        <Widget
          label="Logins"
          icon={<Users className="size-3.5" />}
          value={data.hold === "never" ? "—" : (s?.accounts?.length ?? 0)}
          detail={data.hold === "never" ? "kept on the main machine" : "shared"}
        />
        <Widget
          label="Skills"
          icon={<Sparkles className="size-3.5" />}
          value={skills.data?.shared.length ?? "—"}
          detail={skills.data ? `≈${skills.data.token_estimate.toLocaleString()} tokens` : "in your library"}
        />
      </div>

      {machines.loading && !machines.data ? (
        <GroupSkeleton rows={3} />
      ) : (
        <Group title="Machines">
          {all.map((m) => {
            const t = osTile(m.os);
            return (
              <Row
                key={m.name}
                to="/machines"
                icon={<IconTile icon={t.icon} color={m.online ? t.color : "gray"} />}
                title={
                  <span className="flex items-center gap-2">
                    {m.name}
                    {m.is_self ? <span className="text-xs text-muted">This machine</span> : null}
                  </span>
                }
                subtitle={m.online ? `${osName(m.os)} · ${m.ip ?? ""}` : `Offline · seen ${relative(m.last_seen)}`}
                value={
                  <span className="flex items-center gap-2">
                    {m.leading ? <KeyRound className="size-3.5 text-success" aria-label="holds the accounts" /> : null}
                    {m.hold ? holdName[m.hold] : null}
                    <Dot tone={m.online ? "success" : "muted"} />
                  </span>
                }
              />
            );
          })}
        </Group>
      )}

      <Group title="This Machine">
        <Row title="Name" value={data.name} />
        <Row title="Role" value={holdName[data.hold]} to="/machines" />
        <Row title="CLIProxyAPI" value={s?.proxy_running ? "Running here" : data.hold === "never" ? "Not needed" : "Standing by"} />
        <Row title="Logins synced" value={s?.synced ? secondsAgo(s.synced.seconds_ago) : s?.leading ? "This machine is the source" : "—"} />
        <Row title="Version" value={`${data.version} · ${osName(data.os)}`} />
      </Group>
    </>
  );
}

function NotSetUp() {
  const { reload } = useOverview();
  return (
    <>
      <LargeTitle title="Welcome to spillup" subtitle="Set up this machine with one command, then come back here." />
      <Group
        title="Pick a role"
        footer="Use the same Tailscale account on every machine. Each one asks for a key that the Settings page shows on a machine you already set up."
      >
        <Row icon={<IconTile icon={Crown} color="yellow" />} title="Main machine" subtitle="On the most; holds your accounts" accessory={<Mono copy>spinup setup my-desktop --hub</Mono>} />
        <Row icon={<IconTile icon={RefreshCw} color="blue" />} title="Standby" subtitle="Takes over while the main one is off" accessory={<Mono copy>spinup setup my-laptop --standby</Mono>} />
        <Row icon={<IconTile icon={Terminal} color="gray" />} title="Just use the accounts" subtitle="Anything else" accessory={<Mono copy>spinup setup my-mac</Mono>} />
      </Group>
      <Button variant="secondary" onPress={() => void reload()}>
        Check again
      </Button>
    </>
  );
}
