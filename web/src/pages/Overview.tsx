import { Alert, Button } from "@heroui/react";
import { Crown, KeyRound, Network, RefreshCw, Sparkles, Users } from "lucide-react";
import { useEffect, useState } from "react";
import { useOverview } from "../App";
import { Choice, Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, Widget, holdName, osName, osTile } from "../components/ui";
import type { SetupProgress } from "../lib/types";
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
  const [name, setName] = useState("");
  const [role, setRole] = useState<"hub" | "standby" | "use">("standby");
  const [progress, setProgress] = useState<SetupProgress | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const valid = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(name);

  useEffect(() => {
    if (!progress?.running) return;
    const id = setInterval(async () => {
      const p = await api.setupProgress();
      setProgress(p);
      if (!p.running) void reload();
    }, 1000);
    return () => clearInterval(id);
  }, [progress?.running, reload]);

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
    { id: "hub", icon: Crown, color: "yellow", title: "Main Machine", subtitle: "On the most; holds your accounts." },
    { id: "standby", icon: RefreshCw, color: "blue", title: "Standby", subtitle: "Takes over while the main machine is off. Joins by itself on your Tailscale account." },
    { id: "use", icon: Users, color: "gray", title: "Just Use the Accounts", subtitle: "Uses the main machine's accounts and keeps no copy." },
  ] as const;

  return (
    <>
      <LargeTitle title="Welcome to spillup" subtitle="Set this machine up: spillup installs Tailscale, the accounts service, T3 Code's link and your skills, and starts them at boot." />
      {progress ? (
        <Group
          title={progress.running ? "Setting Up…" : progress.failed ? "Setup Stopped" : "Set Up"}
          footer={progress.failed ? `${progress.failed}. Fix what the log says, then set up again.` : progress.running ? "Approve any administrator prompt, and sign in to Tailscale when the link appears." : undefined}
        >
          <div className="max-h-96 overflow-y-auto px-4 py-3 font-mono text-[12px] leading-relaxed whitespace-pre-wrap">
            {progress.log.length ? progress.log.map((line, i) => <div key={i}>{linkify(line)}</div>) : <span className="text-muted">Starting…</span>}
          </div>
          {!progress.running ? <Row title={progress.failed ? "Set Up Again" : "Done"} tone="accent" onPress={() => (progress.failed ? setProgress(null) : void reload())} /> : null}
        </Group>
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
          {problem ? <p className="mb-4 px-1 text-sm text-danger">{problem}</p> : null}
          <Button isDisabled={!valid} onPress={start}>
            Set Up This Machine
          </Button>
        </>
      )}
    </>
  );
}

/** Links in setup's output, such as Tailscale's sign-in, open in a new tab. */
function linkify(line: string) {
  const parts = line.split(/(https?:\/\/\S+)/g);
  return parts.map((p, i) =>
    /^https?:\/\//.test(p) ? (
      <a key={i} href={p} target="_blank" rel="noopener" className="text-accent underline">
        {p}
      </a>
    ) : (
      p
    ),
  );
}
