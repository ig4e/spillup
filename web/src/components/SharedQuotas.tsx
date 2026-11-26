import type { ReactNode } from "react";
import { BrandTile } from "./brand";
import { Dot, Group, Row } from "./ui";
import { api } from "../lib/api";
import { relative } from "../lib/format";
import { useQuery } from "../lib/hooks";
import { providerLook } from "../lib/providers";
import type { QuotaAccount, QuotaCredit, QuotaWindow } from "../lib/types";

const states = {
  ready: { label: "Ready", tone: "success" },
  disabled: { label: "Disabled", tone: "muted" },
  "sign-in": { label: "Sign-in required", tone: "danger" },
  limited: { label: "Rate limited", tone: "warning" },
  unavailable: { label: "Unavailable", tone: "danger" },
  unknown: { label: "Unknown", tone: "muted" },
} as const;

const usageStatus = {
  refused: "The provider refused the usage read",
  "rate-limited": "The provider rate-limited the usage read",
  unavailable: "The provider's usage couldn't be read",
} as const;

/** "in 2d 10h", "in 4h 59m", "in 12m": precise enough to plan around, short enough for a row. */
function countdown(iso: string) {
  const mins = Math.round((new Date(iso).getTime() - Date.now()) / 60_000);
  if (mins <= 0) return "now";
  const d = Math.floor(mins / 1440), h = Math.floor((mins % 1440) / 60), m = mins % 60;
  if (d) return `in ${d}d${h ? ` ${h}h` : ""}`;
  if (h) return `in ${h}h${m ? ` ${m}m` : ""}`;
  return `in ${m}m`;
}

const when = new Intl.DateTimeFormat(undefined, { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });

function resetText(iso?: string) {
  return iso ? `Resets ${countdown(iso)} · ${when.format(new Date(iso))}` : "";
}

function barTone(w: QuotaWindow) {
  if (w.limited || w.used_percent >= 90) return "bg-danger";
  if (w.used_percent >= 70) return "bg-warning";
  return "bg-success";
}

function Meter({ window: w, showReset = true }: { window: QuotaWindow; showReset?: boolean }) {
  const pct = Math.max(0, Math.min(100, w.used_percent));
  return (
    <div className="min-w-0">
      <div className="flex items-baseline gap-2 text-[13px] lg:text-xs">
        <span className="min-w-0 truncate font-medium text-foreground">{w.label}</span>
        <span className={`ml-auto shrink-0 tabular-nums ${w.limited ? "font-medium text-danger" : "text-muted"}`}>
          {w.limited ? "Limit reached" : `${Math.round(100 - pct)}% left`}
        </span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-foreground/[0.08]" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} aria-label={`${w.label}: ${pct}% used`}>
        <div className={`h-full rounded-full transition-[width] duration-500 ${barTone(w)}`} style={{ width: `${Math.max(pct, pct > 0 ? 2 : 0)}%` }} />
      </div>
      {showReset && w.resets_at ? <div className="mt-1 truncate text-[12px] text-muted tabular-nums lg:text-[11px]">{pct}% used · {resetText(w.resets_at)}</div> : null}
    </div>
  );
}

function amount(value: number, unit: string) {
  if (unit === "credits" || unit === "resets") return new Intl.NumberFormat(undefined, { maximumFractionDigits: value >= 100 ? 0 : 2 }).format(value);
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: unit }).format(value);
  } catch {
    return `${value.toFixed(2)} ${unit}`;
  }
}

function creditValue(c: QuotaCredit) {
  if (c.used !== undefined && c.limit !== undefined) return `${amount(c.used, c.unit)} of ${amount(c.limit, c.unit)}`;
  if (c.balance !== undefined) return c.unit === "resets" ? `${amount(c.balance, c.unit)} available` : `${amount(c.balance, c.unit)} left`;
  if (c.used !== undefined) return `${amount(c.used, c.unit)} used`;
  return "";
}

function Credit({ credit }: { credit: QuotaCredit }) {
  const over = credit.used !== undefined && credit.limit !== undefined && credit.limit > 0 ? (credit.used / credit.limit) * 100 : undefined;
  return (
    <div className="min-w-0">
      <div className="flex items-baseline gap-2 text-[13px] lg:text-xs">
        <span className="min-w-0 truncate font-medium text-foreground">{credit.label}</span>
        <span className="ml-auto shrink-0 tabular-nums text-foreground">{creditValue(credit)}</span>
      </div>
      {over !== undefined ? (
        <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-foreground/[0.08]">
          <div className={`h-full rounded-full ${over >= 100 ? "bg-danger" : over >= 70 ? "bg-warning" : "bg-accent"}`} style={{ width: `${Math.min(100, over)}%` }} />
        </div>
      ) : null}
      {credit.note ? <div className="mt-1 text-[12px] text-muted lg:text-[11px]">{credit.note}</div> : null}
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2.5">
      <div className="text-[11px] font-semibold uppercase tracking-wide text-muted">{title}</div>
      {children}
    </div>
  );
}

/** Models that reset together say so once, instead of on every bar. */
function Models({ models }: { models: QuotaWindow[] }) {
  const resets = new Set(models.map((m) => m.resets_at ?? ""));
  const shared = resets.size === 1 ? models[0]?.resets_at : undefined;
  return (
    <Section title="Models">
      <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
        {models.map((m) => <Meter key={m.label} window={m} showReset={!shared} />)}
      </div>
      {shared ? <div className="text-[12px] text-muted tabular-nums lg:text-[11px]">All models: {resetText(shared)}</div> : null}
    </Section>
  );
}

function QuotaCard({ account, title }: { account: QuotaAccount; title: string }) {
  const provider = providerLook(account.provider);
  const state = states[account.state] ?? states.unknown;
  const windows = account.windows ?? [];
  const models = account.models ?? [];
  const credits = account.credits ?? [];
  const shares = account.shares ?? [];
  const details = windows.length + models.length + credits.length > 0;
  return (
    <div className="flex flex-col gap-3.5 px-4 py-3">
      <div className="flex items-center gap-3">
        <BrandTile brand={account.provider} icon={provider.icon} color={provider.color} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className="min-w-0 break-words text-[16px] leading-snug lg:text-[14px]">{title}</span>
            {account.plan ? <span className="shrink-0 rounded-full bg-foreground/[0.07] px-2 py-0.5 text-[11px] font-medium text-foreground/80">{account.plan}</span> : null}
          </div>
          <div className="text-[13px] text-muted lg:text-xs">
            {details ? `Checked ${relative(account.observed_at)}` : account.usage_status ? usageStatus[account.usage_status] : "This provider reports no usage"}
          </div>
        </div>
        <span className="flex shrink-0 items-center gap-1.5 text-[13px] text-muted sm:text-[14px]"><Dot tone={state.tone} />{state.label}</span>
      </div>
      {windows.length ? (
        <Section title="Limits">
          {windows.map((w) => <Meter key={w.label} window={w} />)}
        </Section>
      ) : null}
      {models.length ? <Models models={models} /> : null}
      {credits.length ? (
        <Section title="Credits">
          {credits.map((c) => <Credit key={c.label} credit={c} />)}
        </Section>
      ) : null}
      {shares.length ? (
        <div className="text-[12px] text-muted lg:text-[11px]">
          This week's use: {shares.map((s) => `${s.label} ${s.percent}%`).join(" · ")}
        </div>
      ) : null}
      {details && account.usage_status ? <div className="text-[12px] text-warning lg:text-[11px]">{usageStatus[account.usage_status]}; showing the last reading.</div> : null}
    </div>
  );
}

export function SharedQuotas() {
  const { data, error, refreshError, loading } = useQuery(api.sharedQuotas, 30_000);
  const accounts = data?.accounts ?? [];
  const seen = new Map<string, number>();
  const counts = new Map<string, number>();
  for (const a of accounts) counts.set(a.provider, (counts.get(a.provider) ?? 0) + 1);
  return (
    <Group
      title="Quotas & Credits"
      footer={data ? `The account holder checked ${relative(data.checked_at)}, at most once every five minutes. ${refreshError ? "Refresh unavailable; showing the last reading." : ""}` : undefined}
    >
      {error ? <Row title="Quotas unavailable" subtitle={<span className="whitespace-normal">{error.message}</span>} />
        : accounts.length ? accounts.map((account) => {
          const n = (seen.get(account.provider) ?? 0) + 1;
          seen.set(account.provider, n);
          const name = providerLook(account.provider).name;
          return <QuotaCard key={`${account.provider}-${n}`} account={account} title={counts.get(account.provider) === 1 ? name : `${name} · Account ${n}`} />;
        }) : <Row title={loading ? "Reading the account holder…" : "No shared accounts"} />}
    </Group>
  );
}
