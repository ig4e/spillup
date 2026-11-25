import { ArrowDown, ArrowUp, Cpu, MemoryStick, Network } from "lucide-react";
import { useMachineStats } from "./Resources";
import { OsTile } from "./brand";
import { openPage, osName, pageHref } from "./ui";
import { useRemote } from "../lib/hooks";
import type { Overview, Sample } from "../lib/types";
import "./overview-activity.css";

const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const time = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit", second: "2-digit" });
const bytes = (n: number) => {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const unit = n > 0 ? Math.min(units.length - 1, Math.max(0, Math.floor(Math.log2(n) / 10))) : 0;
  return `${number.format(n / (1024 ** unit))} ${units[unit]}`;
};
const rate = (n: number) => `${bytes(n)}/s`;
const percent = (n: number) => `${number.format(n)}%`;
const cpu = (s: Sample) => s.cpu;
const memory = (s: Sample) => s.mem_used;
const receive = (s: Sample) => s.rx_rate;
const send = (s: Sample) => s.tx_rate;
const load = (s: Sample) => s.load1;
type Metric = (s: Sample) => number | undefined;
type Reading = { at: number; sample: Sample };
type Line = { metric: Metric; color: string; dashed?: boolean };
const cpuLines = [{ metric: cpu, color: "var(--overview-cpu)" }];
const memoryLines = [{ metric: memory, color: "var(--overview-memory)" }];
const networkLines = [{ metric: receive, color: "var(--overview-rx)" }, { metric: send, color: "var(--overview-tx)", dashed: true }];
const loadLines = [{ metric: load, color: "var(--overview-cpu)" }];

function value(sample: Sample | undefined, metric: Metric, format: (n: number) => string) {
  const n = sample && metric(sample);
  return typeof n === "number" && Number.isFinite(n) && n >= 0 ? format(n) : "—";
}

function Plot({ readings, lines, max, ceiling, empty }: { readings: Reading[]; lines: Line[]; max: number; ceiling: string; empty: string }) {
  const start = readings[0]?.at;
  const end = readings.at(-1)?.at;
  if (start === undefined || end === undefined || start === end || max <= 0) {
    return <div className="overview-plot-empty">{empty}</div>;
  }
  return <div className="overview-plot" aria-hidden="true">
    <div className="overview-scale"><span>{ceiling}</span><span>0</span></div>
    <svg viewBox="0 0 200 54" preserveAspectRatio="none" focusable="false">
      <path className="overview-gridline" d="M0 1H200 M0 27H200 M0 53H200" />
      {lines.map(line => {
        let path = "";
        let previousAt = -Infinity;
        for (const { at, sample } of readings) {
          const n = line.metric(sample);
          if (n === undefined || !Number.isFinite(n) || n < 0) { previousAt = -Infinity; continue; }
          const x = (at - start) / (end - start) * 200;
          const y = 53 - Math.min(n / max, 1) * 52;
          // The sampler runs every two seconds. Leave gaps where a machine slept or collection stopped.
          path += `${at - previousAt > 10_000 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)} `;
          previousAt = at;
        }
        return <path key={line.color} d={path} fill="none" stroke={line.color} strokeWidth="1.9" strokeDasharray={line.dashed ? "4 3" : undefined} vectorEffect="non-scaling-stroke" />;
      })}
    </svg>
  </div>;
}

export function OverviewActivity({ host }: { host: Overview }) {
  const stats = useMachineStats(useRemote());
  const specs = stats.data?.specs;
  const measurements = stats.data?.history ?? [];
  const readings = measurements.map(sample => ({ sample, at: Date.parse(sample.at) })).filter(row => row.at > 0);
  const latest = readings.at(-1);
  const current = latest?.sample;
  const hasUsage = ["windows", "linux"].includes((specs?.os || host.os).toLowerCase());
  const total = specs?.mem_total ?? 0;
  const loadMax = Math.max(1, ...readings.map(({ sample }) => sample.load1 ?? 0));
  const networkMax = Math.max(1, ...readings.map(({ sample }) => Math.max(sample.rx_rate, sample.tx_rate)));
  const window = readings.length > 1 ? `${time.format(readings[0].at)} – ${time.format(latest!.at)}` : latest ? time.format(latest.at) : null;
  const status = stats.error ? "Statistics unavailable" : stats.refreshError ? "Last reading" : window ? "" : "Collecting samples…";
  const waiting = stats.error ? "No reading" : "Collecting history…";
  return (
    <section className="mb-5 min-w-0" aria-label={`Activity on ${host.name}`}>
      <div className="overview-host">
        <OsTile os={host.os} />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[19px] font-semibold tracking-tight" title={host.name}>{host.name || "This machine"}</h2>
          <p className="text-xs text-muted">{osName(host.os)} · {host.version}{specs?.cores ? ` · ${specs.cores} cores` : ""}</p>
        </div>
        <a className="overview-link" href={pageHref("/activity")} onClick={event => openPage(event, "/activity")}>Activity</a>
      </div>
      <div className="overview-charts">
        <figure className="overview-chart">
          <figcaption><Cpu aria-hidden />{hasUsage ? "CPU" : "Load average"}</figcaption>
          <div className="overview-number">{hasUsage ? value(current, cpu, percent) : value(current, load, n => number.format(n))}</div>
          <Plot readings={readings} lines={hasUsage ? cpuLines : loadLines} max={hasUsage ? 100 : loadMax} ceiling={hasUsage ? "100%" : number.format(loadMax)} empty={waiting} />
          {readings.length > 1 ? <span className="sr-only">Peak {hasUsage ? percent(Math.max(...readings.map(r => r.sample.cpu))) : number.format(Math.max(...readings.map(r => r.sample.load1 ?? 0)))}</span> : null}
        </figure>
        <figure className="overview-chart">
          <figcaption><MemoryStick aria-hidden />Memory</figcaption>
          <div className="overview-number">{hasUsage ? value(current, memory, bytes) : total > 0 ? bytes(total) : "—"}</div>
          {hasUsage ? <Plot readings={readings} lines={memoryLines} max={total} ceiling={bytes(total)} empty={total || !latest ? waiting : "Capacity unavailable"} /> : <div className="overview-plot-empty">Installed capacity</div>}
          {hasUsage && total > 0 ? <span className="sr-only">of {bytes(total)} installed</span> : null}
        </figure>
        {hasUsage ? <figure className="overview-chart overview-network">
          <figcaption><Network aria-hidden />Network</figcaption>
          <div className="overview-network-values">
            <span><ArrowDown className="text-[var(--overview-rx)]" aria-hidden /><span className="sr-only">Receive </span>{value(current, receive, rate)}</span>
            <span><ArrowUp className="text-[var(--overview-tx)]" aria-hidden /><span className="sr-only">Send </span>{value(current, send, rate)}</span>
          </div>
          <Plot readings={readings} lines={networkLines} max={networkMax} ceiling={rate(networkMax)} empty={waiting} />
        </figure> : null}
      </div>
      <p className="overview-sample-time">{[status, window].filter(Boolean).join(" · ")}</p>
    </section>
  );
}
