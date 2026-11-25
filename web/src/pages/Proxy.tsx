import { Button } from "../components/Button";
import { ExternalLink, Server } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Group, GroupSkeleton, IconTile, LargeTitle, Row, Segmented } from "../components/ui";
import { api } from "../lib/api";
import { useQuery, useRemote } from "../lib/hooks";

const mocked = import.meta.env.VITE_MOCK === "1";

/** One CLIProxyAPI the page can show: whose it is, and which logins it holds. */
interface Source {
  id: "shared" | "own";
  machine: string;
  /** The machine to fetch it through, or null for this one. */
  via: string | null;
  label: string;
}

/** CLIProxyAPI's own dashboard, window-sized and signed in, with every source named by the machine it runs on. */
export function ProxyPage() {
  const overview = useOverview();
  const data = overview.data;
  const remote = useRemote();
  const local = useQuery(api.localOverview);
  const own = useQuery(api.own);
  const [picked, setPicked] = useState<Source["id"] | null>(null);

  const viewed = data?.name ?? "";
  const here = remote ? local.data?.name : viewed;
  const s = data?.service;
  const holder = s?.leading ? viewed : s?.leader || "";

  const sources: Source[] = [];
  if (holder && here) {
    sources.push({
      id: "shared",
      machine: holder,
      via: holder === here ? null : holder,
      label: "Shared accounts",
    });
  }
  if (viewed && own.data?.running && (here || !holder)) {
    sources.push({
      id: "own",
      machine: viewed,
      via: remote,
      label: "Own sign-ins",
    });
  }
  const source = sources.find((x) => x.id === picked) ?? sources[0];
  const src = source ? `/cpa/${source.id}/${source.via ? `?machine=${encodeURIComponent(source.via)}` : ""}` : "";
  const loading = overview.loading || (remote && local.loading) || own.loading;
  const problem = overview.error?.message ?? data?.service_problem ?? (remote ? local.error?.message : undefined) ?? own.error?.message;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle
        bleed
        title="CLIProxyAPI"
        center={
          sources.length > 1 ? (
            <Segmented label="Which CLIProxyAPI" value={source!.id} onChange={setPicked} options={sources.map((x) => ({ id: x.id, label: x.label }))} />
          ) : undefined
        }
        actions={
          source ? (
            <Button size="sm" variant="secondary" onPress={() => window.open(src, "_blank", "noopener")}>
              <ExternalLink className="size-3.5" /> New Tab
            </Button>
          ) : undefined
        }
      />

      {source ? (
        <p className="mb-3 px-1 text-sm text-muted">
          {source.label} on <strong className="break-all font-medium text-foreground">{source.machine}</strong>
        </p>
      ) : null}

      {!source && loading ? <GroupSkeleton rows={2} /> : !source && problem ? (
        <Group><Row title="Couldn’t locate CLIProxyAPI" subtitle={problem} /></Group>
      ) : !source ? (
        <div className="mx-auto w-full max-w-[640px] pt-10">
          <Group footer="No machine holds the accounts right now, and this machine has no sign-ins of its own. Switch on a provider under Sign-In to give it some.">
            <Row icon={<IconTile icon={Server} color="gray" />} title={<span className="text-muted">No CLIProxyAPI to show</span>} />
          </Group>
        </div>
      ) : mocked ? (
        <div className="flex flex-1 items-center justify-center rounded-xl bg-surface text-sm text-muted shadow-[0_0_0_0.5px_var(--border)]">
          CLIProxyAPI on {source.machine} ({source.id === "shared" ? "shared accounts" : "own sign-ins"}) fills this space.
        </div>
      ) : (
        <iframe
          key={src}
          title={`CLIProxyAPI on ${source.machine}`}
          src={src}
          className="min-h-0 w-full flex-1 rounded-xl bg-surface shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]"
        />
      )}
    </div>
  );
}
