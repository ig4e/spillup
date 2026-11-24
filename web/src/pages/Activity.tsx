import { useMemo, useState } from "react";
import { bytes } from "../components/Resources";
import { IconTile, LargeTitle, Segmented, osName, osTile } from "../components/ui";
import { api } from "../lib/api";
import { useQuery } from "../lib/hooks";
import type { Machine, Sample, Stats } from "../lib/types";

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

/** Every machine's stats, polled together, keyed by name. */
function useFleetStats(machines: Machine[]) {
  const names = machines.map((m) => (m.is_self ? "" : m.name));
  const stats = useQuery(async () => {
    const results = await Promise.allSettled(names.map((n) => api.statsOf(n || null)));
    const out: Record<string, Stats> = {};
    results.forEach((r, i) => {
      if (r.status === "fulfilled") out[machines[i].name] = r.value;
    });
    return out;
  }, 2500);
  return stats.data ?? {};
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
    <div className="flex items-baseline justify-between gap-4 text-[12px]">
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
  const machinesQ = useQuery(api.machines, 10000);
  const machines = (machinesQ.data?.machines ?? []).filter((m) => m.online && (m.is_self || m.hold || m.state));
  const fleet = useFleetStats(machines);
  const [metric, setMetric] = useState<Metric>("cpu");
  const [picked, setPicked] = useState<string | null>(null);
  const selected = machines.find((m) => m.name === picked) ?? machines.find((m) => m.is_self) ?? machines[0];
  const st = selected ? fleet[selected.name] : undefined;

  const processes = useMemo(() => {
    const list = [...(st?.processes ?? [])];
    return metric === "memory" ? list.sort((a, b) => b.mem - a.mem) : list.sort((a, b) => b.cpu - a.cpu || b.mem - a.mem);
  }, [st, metric]);

  const rows = machines.map((m) => ({ m, s: fleet[m.name] }));
  const sortKey = (s?: Stats) =>
    !s ? -1 : metric === "cpu" ? s.now.cpu : metric === "memory" ? s.now.mem_used / (s.specs.mem_total || 1) : metric === "disk" ? (s.now.disk_used ?? 0) / (s.specs.disk_total || 1) : s.now.rx_rate + s.now.tx_rate;
  rows.sort((a, b) => sortKey(b.s) - sortKey(a.s));

  const h: Sample[] = st?.history ?? [];
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle
        bleed
        title="Activity"
        center={<Segmented label="What to show" value={metric} onChange={setMetric} options={metrics} />}
        actions={<span className="text-[12px] text-muted">{machines.length} machines</span>}
      />

      <div className="mb-3 overflow-hidden rounded-xl bg-surface max-lg:overflow-x-auto">
        <table className="w-full text-[13px] max-lg:whitespace-nowrap">
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
            {rows.map(({ m, s }) => {
              const t = osTile(m.os);
              const on = m.name === selected?.name;
              return (
                <tr
                  key={m.name}
                  onClick={() => setPicked(m.name)}
                  className={`cursor-default ${on ? "bg-accent text-white [&_*]:text-white" : "odd:bg-foreground/[0.025] hover:bg-foreground/[0.04]"}`}
                >
                  <td className="px-3 py-1.5">
                    <span className="flex items-center gap-2">
                      <IconTile icon={t.icon} color={t.color} size="sm" />
                      <span className="font-medium">{m.name}</span>
                      <span className="text-xs text-muted">{osName(m.os)}</span>
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
            })}
          </tbody>
        </table>
      </div>

      <div className="mb-3 min-h-0 flex-1 overflow-auto rounded-xl bg-surface">
        <table className="w-full text-[13px] max-lg:whitespace-nowrap">
          <thead>
            <tr>
              <Th>Process Name {selected ? <span className="font-normal">· {selected.name}</span> : null}</Th>
              <Th right active={metric !== "memory"}>% CPU</Th>
              <Th right active={metric === "memory"}>Memory</Th>
              <Th right>PID</Th>
            </tr>
          </thead>
          <tbody>
            {processes.map((p) => (
              <tr key={p.pid} className="odd:bg-foreground/[0.025] hover:bg-foreground/[0.04]">
                <td className="max-w-0 truncate px-3 py-1">{p.name}</td>
                <td className="w-24 px-3 py-1 text-right tabular-nums">{pct(p.cpu)}</td>
                <td className="w-28 px-3 py-1 text-right tabular-nums">{bytes(p.mem)}</td>
                <td className="w-20 px-3 py-1 text-right text-muted tabular-nums">{p.pid}</td>
              </tr>
            ))}
            {!processes.length ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-muted">
                  {st ? "This machine doesn't list its programs." : "Reading…"}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="grid h-[150px] shrink-0 grid-cols-[220px_1fr_220px] grid-rows-[minmax(0,1fr)] gap-4 overflow-hidden rounded-xl bg-surface p-3.5 max-lg:grid-cols-[200px_240px_200px] max-lg:overflow-x-auto">
        {st ? (
          <BottomPanel metric={metric} st={st} h={h} />
        ) : (
          <div className="col-span-3 flex items-center justify-center text-[13px] text-muted">Reading {selected?.name ?? "this machine"}…</div>
        )}
      </div>
    </div>
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
