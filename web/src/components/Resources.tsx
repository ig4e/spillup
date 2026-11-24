import { Cpu, HardDrive, MemoryStick, Timer } from "lucide-react";
import { api } from "../lib/api";
import { useQuery } from "../lib/hooks";
import type { Machine, Sample, Stats } from "../lib/types";
import { Dot, IconTile, osName, osTile } from "./ui";

const unit = ["B", "KB", "MB", "GB", "TB"];

export function bytes(n: number, digits = 1) {
  let i = 0;
  while (n >= 1024 && i < unit.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i === 0 ? 0 : digits)} ${unit[i]}`;
}

const rate = (n: number) => `${bytes(n)}/s`;

function uptime(s: number) {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
}

/** A line of recent values, filled underneath, as in Activity Monitor. */
function Sparkline({ values, max, color, className = "h-8" }: { values: number[]; max?: number; color: string; className?: string }) {
  if (values.length < 2) return <div className={className} />;
  const top = Math.max(max ?? 0, ...values, 1);
  const pts = values.map((v, i) => `${(i / (values.length - 1)) * 100},${100 - (v / top) * 100}`);
  return (
    <svg viewBox="0 0 100 100" preserveAspectRatio="none" className={`w-full ${className}`} aria-hidden>
      <polygon points={`0,100 ${pts.join(" ")} 100,100`} fill={color} opacity={0.14} />
      <polyline points={pts.join(" ")} fill="none" stroke={color} strokeWidth={1.6} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
    </svg>
  );
}

function Meter({ label, icon: Icon, value, detail, color }: { label: string; icon: typeof Cpu; value: number; detail: string; color: string }) {
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div className="min-w-0">
      <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium text-muted">
        <Icon className="size-3" /> {label}
        <span className="ml-auto tabular-nums text-foreground">{Math.round(pct)}%</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-foreground/[0.08]">
        <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${pct}%`, background: color }} />
      </div>
      <div className="mt-1 truncate text-[11px] text-muted tabular-nums">{detail}</div>
    </div>
  );
}

function MachineResources({ machine, viaRemote }: { machine: Machine; viaRemote: string | null }) {
  const stats = useQuery(() => api.statsOf(viaRemote), 3000);
  const t = osTile(machine.os);
  const st: Stats | undefined = stats.data;
  if (stats.error && !st) return null;
  const h: Sample[] = st?.history ?? [];
  const now = st?.now;
  const memPct = st && st.specs.mem_total ? (now!.mem_used / st.specs.mem_total) * 100 : 0;
  const diskPct = st?.specs.disk_total && now?.disk_used ? (now.disk_used / st.specs.disk_total) * 100 : 0;
  return (
    <div className="flex flex-col gap-3.5 rounded-xl bg-surface p-4">
      <div className="flex items-center gap-3">
        <IconTile icon={t.icon} color={t.color} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[14px] font-semibold">
            {machine.name}
            {machine.is_self ? <span className="text-xs font-normal text-muted">This machine</span> : null}
          </div>
          <div className="truncate text-xs text-muted">
            {st ? `${st.specs.cpu_model || osName(st.specs.os)} · ${st.specs.cores} cores · ${bytes(st.specs.mem_total, 0)}` : "Reading…"}
          </div>
        </div>
        {now ? (
          <span className="flex items-center gap-1 text-xs text-muted">
            <Timer className="size-3" /> {uptime(now.uptime)}
          </span>
        ) : null}
      </div>
      {now && st ? (
        <>
          <div className="grid grid-cols-3 gap-4">
            <Meter label="CPU" icon={Cpu} value={now.cpu} detail={now.load1 ? `load ${now.load1.toFixed(2)}` : `${st.specs.cores} cores`} color="var(--accent)" />
            <Meter label="Memory" icon={MemoryStick} value={memPct} detail={`${bytes(now.mem_used)} of ${bytes(st.specs.mem_total, 0)}`} color="#af52de" />
            <Meter label="Disk" icon={HardDrive} value={diskPct} detail={st.specs.disk_total ? `${bytes(now.disk_used ?? 0, 0)} of ${bytes(st.specs.disk_total, 0)}` : "—"} color="#ff9500" />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <div className="mb-1 flex items-center justify-between text-[11px] text-muted">
                <span>CPU</span>
                <span className="tabular-nums">{Math.round(now.cpu)}%</span>
              </div>
              <Sparkline values={h.map((x) => x.cpu)} max={100} color="var(--accent)" />
            </div>
            <div>
              <div className="mb-1 flex items-center justify-between text-[11px] text-muted">
                <span className="flex items-center gap-1.5">
                  <Dot tone="success" className="size-1.5" /> ↓ {rate(now.rx_rate)}
                </span>
                <span className="flex items-center gap-1.5 tabular-nums">
                  <Dot tone="accent" className="size-1.5" /> ↑ {rate(now.tx_rate)}
                </span>
              </div>
              <div className="relative">
                <Sparkline values={h.map((x) => x.rx_rate)} color="#30c759" />
                <div className="absolute inset-0">
                  <Sparkline values={h.map((x) => x.tx_rate)} max={Math.max(...h.map((x) => x.rx_rate), 1)} color="#0a84ff" />
                </div>
              </div>
            </div>
          </div>
          <div className="flex justify-between text-[11px] text-muted tabular-nums">
            <span>Received {bytes(now.rx_total)}</span>
            <span>Sent {bytes(now.tx_total)}</span>
          </div>
        </>
      ) : (
        <div className="h-28 animate-pulse rounded-lg bg-foreground/[0.04]" />
      )}
    </div>
  );
}

/** Live specs and load for every online machine that runs spillup. */
export function Resources({ machines }: { machines: Machine[] }) {
  const shown = machines.filter((m) => m.online && (m.is_self || m.hold || m.state));
  if (!shown.length) return null;
  return (
    <section className="mb-7">
      <h2 className="mb-1.5 px-1 text-[13px] font-semibold text-foreground/80">Resources</h2>
      <div className="grid gap-3 md:grid-cols-2">
        {shown.map((m) => (
          <MachineResources key={m.name} machine={m} viaRemote={m.is_self ? null : m.name} />
        ))}
      </div>
    </section>
  );
}
