import { ChevronRight, ChevronsUpDown } from "lucide-react";
import { useState, type ReactNode } from "react";
import { api } from "../lib/api";
import { activityItems, type ActivityItem, type Outcome } from "../lib/activity";
import { capitalize, relative } from "../lib/format";
import { useQuery } from "../lib/hooks";
import { Group, Row } from "./ui";

type Props = { picked: string | null; setPicked: (name: string) => void };
const picker = "absolute inset-0 cursor-pointer opacity-0";

function Picker({ label, shown, children }: { label: string; shown: string; children: ReactNode }) {
  return <Row title={label} accessory={<span className="flex min-w-0 items-center gap-1 text-muted">
    <span className="truncate">{shown}</span><ChevronsUpDown aria-hidden className="size-3.5 shrink-0" />{children}
  </span>} />;
}

export function ActivityHistory({ picked, setPicked }: Props) {
  const inventory = useQuery(api.localManagedMachines, 15000);
  const hosts = (inventory.data?.machines ?? []).filter(machine => machine.online && !machine.expired);
  const selected = hosts.find(machine => picked ? machine.name === picked : machine.is_self);
  const machinePicker = hosts.length ? <Picker label="Machine" shown={selected?.name ?? "Choose"}>
    <select aria-label="History machine" className={picker} value={selected?.name ?? ""} onChange={e => setPicked(e.target.value)}>
      {!selected ? <option value="">Choose a machine</option> : null}
      {hosts.map(host => <option key={host.name} value={host.name}>{host.name}</option>)}
    </select>
  </Picker> : null;
  return <div className="min-h-0 overflow-auto">
    {selected ? <Records key={selected.name} machine={selected.is_self ? null : selected.name} machinePicker={machinePicker} /> :
      <Group>{machinePicker}<Row title={inventory.loading ? "Checking managed machines…" : inventory.error ? "Machine list unavailable" : "Choose an online managed machine"} /></Group>}
  </div>;
}

function Records({ machine, machinePicker }: { machine: string | null; machinePicker: ReactNode }) {
  const actions = useQuery(api.actionsOf(machine), 5000);
  const jobs = useQuery(api.jobsOf(machine), 5000);
  const [area, setArea] = useState("");
  const [result, setResult] = useState("");
  const records = activityItems(actions.data ?? [], jobs.data ?? []);
  const areas = Array.from(new Set(records.map(row => row.area.name))).sort();
  const filtered = records.filter(row => (!area || row.area.name === area) && (!result || row.outcome === result));
  return <>
    <Group>
      {machinePicker}
      <Picker label="Component" shown={area || "All"}>
        <select aria-label="Activity component" value={area} onChange={e => setArea(e.target.value)} className={picker}>
          <option value="">All components</option>
          {(area && !areas.includes(area) ? [...areas, area] : areas).map(name => <option key={name}>{name}</option>)}
        </select>
      </Picker>
      <Picker label="Outcome" shown={result || "All"}>
        <select aria-label="Activity outcome" value={result} onChange={e => setResult(e.target.value)} className={picker}>
          <option value="">All outcomes</option>
          {(["Failed", "Running", "Completed", "Accepted"] satisfies Outcome[]).map(name => <option key={name}>{name}</option>)}
        </select>
      </Picker>
    </Group>
    <Group title="Recent actions and jobs">
      {actions.error || actions.refreshError ? <Row title="Couldn’t refresh actions" subtitle={(actions.error ?? actions.refreshError)?.message} /> : null}
      {jobs.error || jobs.refreshError ? <Row title="Couldn’t refresh jobs" subtitle={(jobs.error ?? jobs.refreshError)?.message} /> : null}
      {filtered.map(item => <HistoryRow key={item.key} item={item} machine={machine} />)}
      {!filtered.length ? <Row title={actions.loading || jobs.loading ? "Reading history…" : area || result ? "No matching records" : actions.error || jobs.error ? "No records available" : "No recent activity"} /> : null}
    </Group>
  </>;
}

function HistoryRow({ item, machine }: { item: ActivityItem; machine: string | null }) {
  const { action, job } = item;
  const doctor = new URL("/doctor", window.location.origin);
  if (machine) doctor.searchParams.set("machine", machine);
  if (item.area.doctor) doctor.hash = "doctor-" + item.area.doctor;
  const state = job ? capitalize(job.state) : item.outcome === "Failed" ? "Failed" : "Accepted";
  return <details className="group/item px-4">
    <summary className="flex cursor-pointer list-none items-center gap-3 py-2.5 [&::-webkit-details-marker]:hidden">
      <span className="min-w-0 flex-1">
        <span className="block break-words text-[16px] leading-snug lg:text-[14px]">{item.title}</span>
        <time className="mt-0.5 block text-[13px] text-muted lg:text-xs" dateTime={item.at} title={new Date(item.at).toLocaleString()}>{item.area.name} · {relative(item.at)}</time>
      </span>
      <span className={`shrink-0 text-[14px] ${item.outcome === "Failed" ? "text-danger" : "text-muted"}`}>{state}</span>
      <ChevronRight aria-hidden className="size-4 shrink-0 text-muted/60 transition-transform group-open/item:rotate-90" />
    </summary>
    <div className="space-y-1 pb-3 text-[14px] text-muted">
      {action ? <p>{action.origin} · HTTP {action.status} · {action.elapsed_ms} ms</p> : null}
      {job ? <p>Job {job.state}{job.total ? ` · Step ${job.step ?? 0} of ${job.total}` : ""} · Updated {new Date(job.updated).toLocaleString()}</p> : null}
      {action?.id || job?.action_id ? <p className="break-all text-xs text-muted">Action ID <code>{action?.id ?? job?.action_id}</code></p> : null}
      {item.outcome === "Failed" ? <a className="text-accent underline" href={doctor.pathname + doctor.search + doctor.hash}>Open Doctor{item.area.doctor ? ` · ${item.area.doctor === "cliproxyapi" ? "CLIProxyAPI" : item.area.doctor === "t3-code" ? "T3 Code" : capitalize(item.area.doctor)}` : ""}</a> : null}
    </div>
  </details>;
}
