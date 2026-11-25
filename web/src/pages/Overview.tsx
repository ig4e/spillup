import { Alert } from "@heroui/react";
import { SetupPreview } from "../components/SetupPreview";
import { Button } from "../components/Button";
import { Crown, KeyRound, Network, RefreshCw, Sparkles, Users } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Choice, Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, Widget, holdName, osName } from "../components/ui";
import { OsTile } from "../components/brand";
import { TaskLog } from "../components/TaskProgress";
import { api } from "../lib/api";
import { relative, secondsAgo } from "../lib/format";
import { useQuery, useTaskProgress } from "../lib/hooks";

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
      <LargeTitle title="Overview" />

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
        <div className="mt-0.5 max-w-full break-words text-[34px] leading-tight font-bold tracking-[-0.025em]">{leader || "no machine yet"}</div>
        <div className="mt-2 flex items-center gap-1.5 text-[13px] text-muted">
          <Dot tone={leader ? "success" : "warning"} />
          {leader ? (s?.in_flight ? `Serving ${s.in_flight} request${s.in_flight === 1 ? "" : "s"}` : "Ready") : "Waiting for the main machine"}
        </div>
        <div className="mt-4 max-w-full text-[13px] text-muted">
          {data.router_url ? <>Tailnet proxy at <Mono copy>{data.router_url}</Mono></> : "Tailnet proxy address unavailable"}
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
            return (
              <Row
                key={m.name}
                to="/machines"
                icon={<OsTile os={m.os} dim={!m.online} />}
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
  const [name, setName] = useState("");
  const [role, setRole] = useState<"hub" | "standby" | "use">("standby");
  const { progress, setProgress, connectionError } = useTaskProgress("setup");
  const [problem, setProblem] = useState<string | null>(null);
  const valid = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(name);


  const start = async () => {
    try {
      await api.setup(name, role);
      setProblem(null);
      setProgress({ running: true, done: false, log: [] });
    } catch (e) {
      setProblem((e as Error).message);
    }
  };

  const roles = [
    { id: "hub", icon: Crown, color: "yellow", title: "Main", subtitle: "Works, and holds the logins for every machine. Pick the one that’s on the most." },
    { id: "standby", icon: RefreshCw, color: "blue", title: "Worker + Standby", subtitle: "Works with the accounts, and takes over holding them while the main machine is off." },
    { id: "use", icon: Users, color: "gray", title: "Worker", subtitle: "Works with the accounts through the main machine and keeps no copy of the logins." },
  ] as const;

  return (
    <>
      <LargeTitle title="Welcome to spillup" subtitle="Every machine works with your accounts at the same time; one of them holds the logins. Set this machine up and spillup installs Tailscale, the accounts service, T3 Code's link and your skills, and starts them at boot." />
      {progress ? (
        <TaskLog
          progress={progress}
          connectionError={connectionError}
          hint="Approve any administrator prompt, and sign in to Tailscale when the link appears."
          onRetry={() => setProgress(null)}
          onDone={() => void reload()}
        />
      ) : (
        <>
          <Group title="This Machine's Name" footer="Its address on your network: lowercase letters, digits and dashes.">
            <div className="px-4 py-2.5">
              <input
                aria-label="This machine's name"
                value={name}
                onChange={(e) => setName(e.target.value.toLowerCase())}
                placeholder="my-desktop"
                className="w-full bg-transparent text-[14px] outline-none placeholder:text-muted"
              />
            </div>
          </Group>
          <Group title="Role">
            {roles.map((r) => (
              <Choice key={r.id} icon={<IconTile icon={r.icon} color={r.color} />} title={r.title} subtitle={r.subtitle} chosen={role === r.id} onChoose={() => setRole(r.id)} />
            ))}
          </Group>
          {valid ? <SetupPreview key={`${name}/${role}`} name={name} role={role} /> : null}
          {problem ? <p className="mb-4 px-1 text-sm text-danger">{problem}</p> : null}
          <Button isDisabled={!valid} onPress={start}>
            Set Up This Machine
          </Button>
        </>
      )}
    </>
  );
}
