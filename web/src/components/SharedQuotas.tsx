import { BrandTile } from "./brand";
import { Dot, Group, Row } from "./ui";
import { api } from "../lib/api";
import { relative } from "../lib/format";
import { useQuery } from "../lib/hooks";
import { providerLook } from "../lib/providers";
import type { QuotaSummary } from "../lib/types";

const states = {
  ready: { label: "Ready", tone: "success" },
  disabled: { label: "Disabled", tone: "muted" },
  "sign-in": { label: "Sign-in required", tone: "danger" },
  limited: { label: "Rate limited", tone: "warning" },
  unavailable: { label: "Unavailable", tone: "danger" },
  unknown: { label: "Unknown", tone: "muted" },
} as const;

const usageStatus = {
  refused: "Provider refused the quota read",
  "rate-limited": "Provider rate-limited the quota read",
  unavailable: "Provider usage could not be read",
} as const;

function QuotaRow({ account, title }: { account: QuotaSummary["accounts"][number]; title: string }) {
  const provider = providerLook(account.provider);
  const state = states[account.state] ?? states.unknown;
  return (
    <Row
      icon={<BrandTile brand={account.provider} icon={provider.icon} color={provider.color} />}
      title={title}
      value={<span className="flex items-center gap-1.5"><Dot tone={state.tone} />{state.label}</span>}
      subtitle={
        account.windows?.length ? (
          <span className="flex flex-col whitespace-normal">
            {account.windows.map((w, i) => (
              <span key={i}>
                {w.label} · {w.used_percent}% used{w.resets_at ? ` · reset ${new Date(w.resets_at).toLocaleString()}` : ""}
              </span>
            ))}
            <span>Observed {relative(account.observed_at)}</span>
            {account.usage_status ? <span>{usageStatus[account.usage_status]}</span> : null}
          </span>
        ) : account.usage_status ? usageStatus[account.usage_status] : "No quota window reported"
      }
    />
  );
}

export function SharedQuotas() {
  const { data, error, refreshError, loading } = useQuery(api.sharedQuotas, 30_000);
  const accounts = data?.accounts ?? [];
  const seen = new Map<string, number>();
  const counts = new Map<string, number>();
  for (const a of accounts) counts.set(a.provider, (counts.get(a.provider) ?? 0) + 1);
  return (
    <Group title="Shared Quota & Upstream Health" footer={data ? `Holder checked ${relative(data.checked_at)}. ${refreshError ? "Refresh unavailable. Showing the last reading." : "Claude and Codex usage is checked up to once every five minutes."}` : undefined}>
      {error ? <Row title="Quota summary unavailable" subtitle={<span className="whitespace-normal">{error.message}</span>} />
        : accounts.length ? accounts.map((account) => {
          const n = (seen.get(account.provider) ?? 0) + 1;
          seen.set(account.provider, n);
          const name = providerLook(account.provider).name;
          return <QuotaRow key={`${account.provider}-${n}`} account={account} title={counts.get(account.provider) === 1 ? name : `${name} · Account ${n}`} />;
        }) : <Row title={loading ? "Reading the account holder…" : "No shared accounts"} />}
    </Group>
  );
}
