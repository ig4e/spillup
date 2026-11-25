import { AlertDialog, toast } from "@heroui/react";
import { CircleX } from "lucide-react";
import { useMemo, useState } from "react";
import { Button } from "../components/Button";
import { bytes, useMachineStats } from "../components/Resources";
import { Group, LargeTitle, Row, Segmented, osName } from "../components/ui";
import { OsTile } from "../components/brand";
import { api } from "../lib/api";
import { ActivityHistory } from "../components/ActivityHistory";
import { useQuery, useRemote } from "../lib/hooks";
import type { Machine, Process, Sample, Stats } from "../lib/types";

type Metric = "cpu" | "memory" | "disk" | "network";

const metrics: { id: Metric; label: string }[] = [
  { id: "cpu", label: "CPU" },
  { id: "memory", label: "Memory" },
  { id: "disk", label: "Disk" },
  { id: "network", label: "Network" },
];

const rate = (n: number) => `${bytes(n)}/s`;
const pct = (n: number) => `${n.toFixed(1)}`;

function uptime(s: number) {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
}

/** A graph like Activity Monitor's bottom panel: one or two filled lines over a faint grid. */
function Graph({ series, max }: { series: { values: number[]; color: string }[]; max?: number }) {
  const top = Math.max(max ?? 0, ...series.flatMap((s) => s.values), 1);
  return (
    <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="h-full w-full" aria-hidden>
      {[25, 50, 75].map((y) => (
        <line key={y} x1="0" x2="100" y1={y} y2={y} stroke="currentColor" strokeOpacity={0.08} strokeWidth={0.5} vectorEffect="non-scaling-stroke" />
      ))}
      {series.map((s, i) => {
        if (s.values.length < 2) return null;
        const pts = s.values.map((v, j) => `${(j / (s.values.length - 1)) * 100},${100 - (v / top) * 96}`);
        return (
          <g key={i}>
            <polygon points={`0,100 ${pts.join(" ")} 100,100`} fill={s.color} opacity={0.18} />
            <polyline points={pts.join(" ")} fill="none" stroke={s.color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          </g>
        );
      })}
    </svg>
  );
}

function Stat({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="flex min-w-0 items-baseline justify-between gap-1 text-[12px]">
      <span className="flex items-center gap-1.5 text-muted">
        {color ? <span className="size-2 rounded-sm" style={{ background: color }} /> : null}
        {label}
      </span>
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  );
}

function Th({ children, right, active }: { children: React.ReactNode; right?: boolean; active?: boolean }) {
  return (
    <th className={`sticky top-0 z-[1] bg-surface px-3 py-1.5 text-[11px] font-semibold ${right ? "text-right" : "text-left"} ${active ? "text-foreground" : "text-muted"}`}>
      {children}
    </th>
  );
}

export function ActivityPage() {
  const [showActions, setShowActions] = useState(false);
  const remote = useRemote();
  const [picked, setPicked] = useState<string | null>(remote);
  const [metric, setMetric] = useState<Metric>("cpu");
  if (!showActions) return (
    <ActivityMonitor picked={picked} setPicked={setPicked} metric={metric} setMetric={setMetric} onShowHistory={() => setShowActions(true)} />
  );
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle bleed title="Activity" actions={<Button size="sm" variant="secondary" onPress={() => setShowActions(false)}>Processes</Button>} />
      <ActivityHistory picked={picked} setPicked={setPicked} />
    </div>
  );
}

interface MonitorProps {
  picked: string | null;
  setPicked: (name: string) => void;
  metric: Metric;
  setMetric: (metric: Metric) => void;
  onShowHistory: () => void;
}

function ActivityMonitor(props: MonitorProps) {
  const machinesQ = useQuery(api.localManagedMachines, 15000);
  const machines = (machinesQ.data?.machines ?? []).filter((m) => m.online && !m.expired);
  const selected = props.picked ? machines.find((m) => m.name === props.picked) : machines.find((m) => m.is_self);
  if (!selected) return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle bleed title="Activity" />
      <Group title={machinesQ.loading ? "Checking managed machines…" : "Choose a managed machine"}>
        {machinesQ.error ? <Row title="Machine list unavailable" subtitle={machinesQ.error.message} /> : null}
        {machines.map(m => <Row key={m.name} title={m.name} onPress={() => props.setPicked(m.name)} />)}
        {machinesQ.data && !machines.length ? <Row title="No managed machines online" /> : null}
      </Group>
    </div>
  );
  return <ProcessList {...props} selected={selected} machines={machines} verified={!machinesQ.refreshError && !machinesQ.data?.problem} />;
}

function ProcessList({ selected, machines, verified, setPicked, metric, setMetric, onShowHistory }: MonitorProps & {
  selected: Machine;
  machines: Machine[];
  verified: boolean;
}) {
  const [chosen, setChosen] = useState<number | null>(null);
  const [ending, setEnding] = useState<{ process: Process; target: string | null } | null>(null);
  const target = selected.is_self ? null : selected.name;
  const stats = useMachineStats(target);
  const st = stats.data;
  const live = verified && !!st && !stats.refreshError;

  const processes = useMemo(() => {
    const list = [...(st?.processes ?? [])];
    return metric === "memory" ? list.sort((a, b) => b.mem - a.mem) : list.sort((a, b) => b.cpu - a.cpu || b.mem - a.mem);
  }, [st, metric]);

  const chosenProcess = processes.find((p) => p.pid === chosen) ?? null;

  const h: Sample[] = st?.history ?? [];
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle
        bleed
        title="Activity"
        center={<Segmented label="What to show" value={metric} onChange={setMetric} options={metrics} />}
        actions={
          <div className="flex gap-2">
            <Button size="sm" variant="secondary" onPress={onShowHistory}>Recent Actions</Button>
            <button
              type="button"
              aria-label="End the selected process"
              title="End the selected process"
              disabled={!chosenProcess || !live}
              onClick={() => chosenProcess && live && setEnding({ process: chosenProcess, target })}
              className="flex size-8 items-center justify-center rounded-full text-foreground outline-none hover:bg-foreground/[0.06] focus-visible:ring-2 focus-visible:ring-accent/60 disabled:text-muted disabled:opacity-40 disabled:hover:bg-transparent"
            >
              <CircleX className="size-[18px]" />
            </button>
          </div>
        }
      />

      <div className="mb-3 shrink-0 overflow-auto rounded-xl bg-surface max-sm:max-h-[26dvh]">
        <table data-metric={metric} className="activity-machines w-full text-[13px]">
          <thead>
            <tr>
              <Th>Machine</Th>
              <Th right active={metric === "cpu"}>% CPU</Th>
              <Th right active={metric === "memory"}>Memory</Th>
              <Th right active={metric === "disk"}>Disk</Th>
              <Th right active={metric === "network"}>Received</Th>
              <Th right active={metric === "network"}>Sent</Th>
              <Th right>Uptime</Th>
            </tr>
          </thead>
          <tbody>
            {machines.map((m) => (
              <MachineRow key={m.name} m={m} on={m.name === selected?.name} onPick={() => {
                setPicked(m.name);
                setChosen(null);
              }} />
            ))}
          </tbody>
        </table>
      </div>

      <div className="mb-3 min-h-0 flex-1 overflow-auto rounded-xl bg-surface">
        <table className="activity-programs w-full text-[13px]">
          <thead>
            <tr>
              <Th>Process Name <span className="font-normal">· {selected.name}{stats.refreshError && st ? " · Last reading" : ""}</span></Th>
              <Th right active={metric !== "memory"}>% CPU</Th>
              <Th right active={metric === "memory"}>Memory</Th>
              <Th right>PID</Th>
            </tr>
          </thead>
          <tbody>
            {processes.map((p) => (
              <tr
                key={p.pid}
                aria-selected={p.pid === chosen}
                onClick={() => setChosen(p.pid === chosen ? null : p.pid)}
                onDoubleClick={() => live && setEnding({ process: p, target })}
                className={`cursor-default select-none ${p.pid === chosen ? "bg-accent text-white [&_td]:text-white" : "odd:bg-foreground/[0.025] hover:bg-foreground/[0.04]"}`}
              >
                <td className="max-w-0 truncate px-3 py-1" title={p.name}>{p.name}<span className="block text-[10px] text-muted sm:hidden">PID {p.pid}</span></td>
                <td className="w-24 px-3 py-1 text-right tabular-nums">{pct(p.cpu)}</td>
                <td className="w-28 px-3 py-1 text-right tabular-nums">{bytes(p.mem)}</td>
                <td className="w-20 px-3 py-1 text-right text-muted tabular-nums">{p.pid}</td>
              </tr>
            ))}
            {!processes.length ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-muted">
                  {st ? "This machine doesn't list its programs." : stats.loading ? "Reading…" : "Machine statistics are unavailable."}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="activity-details grid h-[150px] shrink-0 grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] gap-4 rounded-xl bg-surface p-3.5">
        {st ? (
          <BottomPanel metric={metric} st={st} h={h} />
        ) : (
          <div className="col-span-3 flex items-center justify-center text-[13px] text-muted">{stats.loading ? "Reading statistics…" : "Machine statistics are unavailable."}</div>
        )}
      </div>
      <EndDialog
        process={ending?.process ?? null}
        machine={ending?.target ?? null}
        available={live && ending?.target === target && processes.some((p) => p.pid === ending?.process.pid && p.name === ending.process.name)}
        onClose={(ended) => {
          setEnding(null);
          if (ended) setChosen(null);
        }}
      />
    </div>
  );
}

function MachineRow({ m, on, onPick }: { m: Machine; on: boolean; onPick: () => void }) {
  const stats = useMachineStats(m.is_self ? null : m.name);
  const s = stats.data;
  return (
    <tr
      onClick={onPick}
      className={`cursor-default ${on ? "bg-accent text-white [&_*]:text-white" : "odd:bg-foreground/[0.025] hover:bg-foreground/[0.04]"}`}
    >
      <td className="px-3 py-1.5">
        <span className="flex items-center gap-2">
          <OsTile os={m.os} size="sm" />
          <button type="button" className="min-w-0 break-all rounded-sm text-left font-medium focus-visible:outline-accent" aria-pressed={on}>{m.name}</button>
          <span className="text-xs text-muted max-sm:hidden">{stats.refreshError ? "Last reading" : osName(m.os)}</span>
        </span>
      </td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s ? pct(s.now.cpu) : "—"}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s ? bytes(s.now.mem_used) : "—"}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s?.now.disk_used ? bytes(s.now.disk_used, 0) : "—"}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s ? rate(s.now.rx_rate) : "—"}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s ? rate(s.now.tx_rate) : "—"}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{s ? uptime(s.now.uptime) : "—"}</td>
    </tr>
  );
}

function BottomPanel({ metric, st, h }: { metric: Metric; st: Stats; h: Sample[] }) {
  const n = st.now;
  const accent = "#0a84ff";
  if (metric === "cpu") {
    return (
      <>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="CPU" value={`${pct(n.cpu)}%`} color={accent} />
          <Stat label="Idle" value={`${pct(100 - n.cpu)}%`} />
          {n.load1 ? <Stat label="Load (1 min)" value={n.load1.toFixed(2)} /> : null}
        </div>
        <div className="flex min-h-0 flex-col">
          <div className="mb-1 text-center text-[11px] font-semibold text-muted">CPU LOAD</div>
          <div className="min-h-0 flex-1">
            <Graph series={[{ values: h.map((x) => x.cpu), color: accent }]} max={100} />
          </div>
        </div>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="Cores" value={String(st.specs.cores)} />
          <Stat label="Processes" value={String(st.process_count)} />
          <div className="truncate text-[11px] text-muted" title={st.specs.cpu_model}>
            {st.specs.cpu_model}
          </div>
        </div>
      </>
    );
  }
  if (metric === "memory") {
    const purple = "#af52de";
    const used = st.specs.mem_total ? (n.mem_used / st.specs.mem_total) * 100 : 0;
    return (
      <>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="Physical Memory" value={bytes(st.specs.mem_total, 0)} />
          <Stat label="Memory Used" value={bytes(n.mem_used)} color={purple} />
          <Stat label="Available" value={bytes(st.specs.mem_total - n.mem_used)} />
        </div>
        <div className="flex min-h-0 flex-col">
          <div className="mb-1 text-center text-[11px] font-semibold text-muted">MEMORY USED</div>
          <div className="min-h-0 flex-1">
            <Graph series={[{ values: h.map((x) => x.mem_used), color: purple }]} max={st.specs.mem_total} />
          </div>
        </div>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="In use" value={`${pct(used)}%`} />
          <Stat label="Processes" value={String(st.process_count)} />
        </div>
      </>
    );
  }
  if (metric === "disk") {
    const orange = "#ff9500";
    const total = st.specs.disk_total ?? 0;
    const used = n.disk_used ?? 0;
    return (
      <>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="Capacity" value={total ? bytes(total, 0) : "—"} />
          <Stat label="Used" value={used ? bytes(used) : "—"} color={orange} />
          <Stat label="Free" value={total ? bytes(total - used) : "—"} />
        </div>
        <div className="flex min-h-0 flex-col">
          <div className="mb-1 text-center text-[11px] font-semibold text-muted">DISK USED</div>
          <div className="min-h-0 flex-1">
            <Graph series={[{ values: h.map((x) => x.disk_used ?? 0), color: orange }]} max={total} />
          </div>
        </div>
        <div className="flex flex-col justify-center gap-1.5">
          <Stat label="In use" value={total ? `${pct((used / total) * 100)}%` : "—"} />
        </div>
      </>
    );
  }
  const green = "#30c759";
  return (
    <>
      <div className="flex flex-col justify-center gap-1.5">
        <Stat label="Data received/sec" value={rate(n.rx_rate)} color={green} />
        <Stat label="Data received" value={bytes(n.rx_total)} />
      </div>
      <div className="flex min-h-0 flex-col">
        <div className="mb-1 text-center text-[11px] font-semibold text-muted">DATA</div>
        <div className="min-h-0 flex-1">
          <Graph
            series={[
              { values: h.map((x) => x.rx_rate), color: green },
              { values: h.map((x) => x.tx_rate), color: accent },
            ]}
          />
        </div>
      </div>
      <div className="flex flex-col justify-center gap-1.5">
        <Stat label="Data sent/sec" value={rate(n.tx_rate)} color={accent} />
        <Stat label="Data sent" value={bytes(n.tx_total)} />
      </div>
    </>
  );
}

/** Activity Monitor's question before it ends a program: quit it politely, or force it. */
function EndDialog({ process, machine, available, onClose }: { process: Process | null; machine: string | null; available: boolean; onClose: (ended: boolean) => void }) {
  const [busy, setBusy] = useState<"quit" | "force" | null>(null);
  const end = async (force: boolean) => {
    if (!process || !available) return;
    setBusy(force ? "force" : "quit");
    try {
      const done = await api.endProcess(machine, process.pid, force);
      toast.success(done.message);
      onClose(true);
    } catch (e) {
      toast.danger((e as Error).message);
    } finally {
      setBusy(null);
    }
  };
  return (
    <AlertDialog.Backdrop isOpen={!!process} onOpenChange={(open) => !open && !busy && onClose(false)}>
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-[420px]">
          <AlertDialog.Header>
            <AlertDialog.Icon status="warning" />
            <AlertDialog.Heading>Are you sure you want to quit “{process?.name}”?</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="text-sm text-muted">
              {available ? <>Quit asks it to close{machine ? ` on ${machine}` : ""}, so it can save its work. Force Quit ends it immediately and can discard unsaved work.</> : "Waiting for this process to appear in the machine’s live readings."}
            </div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button variant="tertiary" isDisabled={!!busy} onPress={() => onClose(false)}>
              Cancel
            </Button>
            <Button variant="danger-soft" isPending={busy === "force"} isDisabled={!!busy || !available} onPress={() => void end(true)}>
              <>Force Quit</>
            </Button>
            <Button isPending={busy === "quit"} isDisabled={!!busy || !available} onPress={() => void end(false)}>
              <>Quit</>
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  );
}
