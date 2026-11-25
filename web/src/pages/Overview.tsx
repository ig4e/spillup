import { Alert } from "@heroui/react";
import { SetupPreview } from "../components/SetupPreview";
import { Button } from "../components/Button";
import { Crown, KeyRound, RefreshCw, Users } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Choice, Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, holdName, openPage, osName, pageHref } from "../components/ui";
import { OsTile } from "../components/brand";
import { OverviewActivity } from "../components/OverviewActivity";
import { TaskLog } from "../components/TaskProgress";
import { api } from "../lib/api";
import { secondsAgo } from "../lib/format";
import { useQuery, useTaskProgress } from "../lib/hooks";
import type { Overview } from "../lib/types";

export function OverviewPage() {
  const overview = useOverview();
  if (overview.data && !overview.data.installed) return <NotSetUp />;
  return <>
    <LargeTitle title="Overview" />
    {overview.data ? <InstalledOverview host={overview.data} stale={!!overview.refreshError} /> : overview.loading ? <GroupSkeleton rows={4} /> : <Group><Row title="Machine status unavailable" subtitle={overview.error?.message} /></Group>}
  </>;
}

function InstalledOverview({ host, stale }: { host: Overview; stale: boolean }) {
  const machines = useQuery(api.localManagedMachines, 15_000);
  const skills = useQuery(api.skills);
  const report = host.service;
  const holder = report?.leading ? host.name : report?.leader;
  const status = !report ? "Unavailable" : report.waiting ? "Waiting" : report.starting ? "Starting" :
    report.leading ? (report.proxy_running ? "Serving here" : "Proxy stopped") : holder ? "Using shared accounts" : "Awaiting holder";
  const healthy = !!report && !report.waiting && !report.starting && (report.leading ? report.proxy_running : !!holder);
  const all = machines.data?.machines ?? [];

  return <>
    {host.service_problem || report?.waiting ? (
      <Alert status={host.service_problem ? "danger" : "warning"} className="mb-4">
        <Alert.Indicator /><Alert.Content>
          <Alert.Title>{host.service_problem ? "Accounts service unavailable" : "Accounts service waiting"}</Alert.Title>
          <Alert.Description>{host.service_problem || report?.waiting}</Alert.Description>
        </Alert.Content>
      </Alert>
    ) : null}
    <OverviewActivity host={host} />
    <section className="mb-5 min-w-0" aria-label="Accounts service">
      <div className="overview-service-heading">
        <h2 className="flex-1 text-[16px] font-semibold">Accounts service</h2>
        <a className="overview-link" href={pageHref("/accounts")} onClick={event => openPage(event, "/accounts")}>Accounts</a>
      </div>
      <div className="group-list overflow-hidden rounded-xl bg-surface">
        <Row icon={<IconTile icon={KeyRound} color={healthy && !stale ? "green" : "gray"} />}
          title={holder || "No account holder reported"}
          subtitle={stale ? "Last reported holder" : "Account holder"}
          accessory={<span className="flex items-center gap-1.5 text-right text-xs text-muted"><Dot tone={stale ? "muted" : healthy ? "success" : "warning"} />{status}</span>} />
        <Row title="Proxy endpoint" value={host.router_url ? <Mono copy>{host.router_url}</Mono> : "Unavailable"} />
        {report ? <Row title="Requests on this machine" value={report.in_flight ?? 0} /> : null}
        {report?.leading ? <Row title="Shared logins" value={report.accounts?.length ?? 0} /> :
          report?.synced ? <Row title="Last account sync" value={secondsAgo(report.synced.seconds_ago)} /> : null}
      </div>
    </section>

    <Group title="Managed machines" footer={machines.refreshError ? "Showing the last verified machine list." : undefined}>
      {all.length ? all.map(m => <Row key={m.name} icon={<OsTile os={m.os} />} title={m.name}
        subtitle={[osName(m.os), m.ip].filter(Boolean).join(" · ")}
        value={m.hold ? holdName[m.hold] : "Managed"} />) :
        <Row title={machines.loading ? "Checking machines…" : machines.error ? "Machine list unavailable" : "No managed machines reported"} />}
    </Group>
    <Group title="Library">
      <Row title="Shared skills" value={skills.data?.shared.filter(s => !s.private).length ?? "—"}
        subtitle={skills.data ? `Estimated ${skills.data.token_estimate.toLocaleString()} tokens` : skills.error ? "Library unavailable" : undefined}
        to="/skills" />
    </Group>
  </>;
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
