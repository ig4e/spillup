import { Tabs } from "@heroui/react";
import { Button } from "../components/Button";
import { Crown, ExternalLink, LogIn, Server, Users } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Group, IconTile, Row } from "../components/ui";
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
  what: string;
}

/** CLIProxyAPI's own dashboard, window-sized and signed in, with every source named by the machine it runs on. */
export function ProxyPage() {
  const { data } = useOverview();
  const remote = useRemote();
  const local = useQuery(api.localOverview);
  const own = useQuery(api.own);
  const [picked, setPicked] = useState<Source["id"] | null>(null);

  const viewed = data?.name ?? "";
  const here = local.data?.name ?? "";
  const s = data?.service;
  const holder = s?.leading ? viewed : s?.leader || "";
  const hubOnly = !!s?.hub_only;

  const sources: Source[] = [];
  if (holder) {
    sources.push({
      id: "shared",
      machine: holder,
      via: holder === here ? null : holder,
      label: `Shared accounts · ${holder}`,
      what: `The logins every machine uses, held by ${holder}${holder === here ? " (this machine)" : ""}. Changes here apply to every machine.`,
    });
  }
  if (own.data?.running) {
    sources.push({
      id: "own",
      machine: viewed,
      via: remote,
      label: `Own sign-ins · ${viewed}`,
      what: `Sign-ins only ${viewed} uses, for the providers switched on under Sign-In. Nothing syncs them.`,
    });
  }
  const source = sources.find((x) => x.id === picked) ?? sources[0];
  const src = source ? `/cpa/${source.id}/${source.via ? `?machine=${encodeURIComponent(source.via)}` : ""}` : "";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <h1 className="text-[20px] font-bold tracking-tight">CLIProxyAPI</h1>
        <span className="flex items-center gap-1.5 rounded-full bg-surface px-2.5 py-1 text-xs text-muted shadow-[0_0_0_0.5px_var(--border)]">
          {hubOnly ? <Crown className="size-3.5 text-warning" /> : <Users className="size-3.5 text-success" />}
          {hubOnly ? "Main machine only" : "Copied to standbys"}
        </span>
        <div className="ml-auto flex items-center gap-2">
          {sources.length > 1 ? (
            <Tabs selectedKey={source?.id} onSelectionChange={(k) => setPicked(String(k) as Source["id"])} variant="primary">
              <Tabs.ListContainer>
                <Tabs.List aria-label="Which CLIProxyAPI">
                  {sources.map((x) => (
                    <Tabs.Tab key={x.id} id={x.id} className="px-3 text-[13px] whitespace-nowrap">
                      {x.label}
                      <Tabs.Indicator />
                    </Tabs.Tab>
                  ))}
                </Tabs.List>
              </Tabs.ListContainer>
            </Tabs>
          ) : null}
          {source ? (
            <Button size="sm" variant="secondary" onPress={() => window.open(src, "_blank", "noopener")}>
              <ExternalLink className="size-3.5" /> New Tab
            </Button>
          ) : null}
        </div>
      </div>

      {source ? (
        <div className="mb-3 flex items-center gap-3 rounded-xl bg-surface px-3.5 py-2.5 shadow-[0_0_0_0.5px_var(--border)]">
          <IconTile icon={source.id === "shared" ? Server : LogIn} color={source.id === "shared" ? "teal" : "pink"} size="sm" />
          <div className="min-w-0 flex-1 text-[13px]">
            <span className="font-semibold">
              Showing CLIProxyAPI on {source.machine}
              {source.machine === here ? " (this machine)" : ""}
            </span>
            <span className="text-muted"> — {source.what}</span>
          </div>
        </div>
      ) : null}

      {!source ? (
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
