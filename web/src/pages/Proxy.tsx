import { Button, Tabs } from "@heroui/react";
import { ExternalLink, Server } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Group, IconTile, Row } from "../components/ui";
import { api } from "../lib/api";
import { useQuery, useRemote } from "../lib/hooks";

type Target = "shared" | "own";

const mocked = import.meta.env.VITE_MOCK === "1";

/** CLIProxyAPI's own dashboard, as big as the window, served through spillup so it opens signed in; for
 * another machine, that machine's own. */
export function ProxyPage() {
  const { data } = useOverview();
  const remote = useRemote();
  const own = useQuery(api.own);
  const canShared = !!data?.can_manage;
  const canOwn = !!own.data?.running;
  const [picked, setPicked] = useState<Target | null>(null);
  const target: Target | null = picked ?? (canShared ? "shared" : canOwn ? "own" : null);
  const src = target ? `/cpa/${target}/${remote ? `?machine=${encodeURIComponent(remote)}` : ""}` : "";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <h1 className="text-[20px] font-bold tracking-tight">CLIProxyAPI</h1>
        <span className="text-[13px] text-muted">{remote ? `on ${remote}` : data?.name ? `on ${data.name}` : ""}</span>
        <div className="ml-auto flex items-center gap-2">
          {canShared && canOwn ? (
            <Tabs selectedKey={target ?? "shared"} onSelectionChange={(k) => setPicked(String(k) as Target)} variant="primary">
              <Tabs.ListContainer>
                <Tabs.List aria-label="Which CLIProxyAPI">
                  <Tabs.Tab id="shared" className="px-3 text-[13px] whitespace-nowrap">
                    Shared Accounts
                    <Tabs.Indicator />
                  </Tabs.Tab>
                  <Tabs.Tab id="own" className="px-3 text-[13px] whitespace-nowrap">
                    Own Sign-Ins
                    <Tabs.Indicator />
                  </Tabs.Tab>
                </Tabs.List>
              </Tabs.ListContainer>
            </Tabs>
          ) : null}
          {target ? (
            <Button size="sm" variant="secondary" onPress={() => window.open(src, "_blank", "noopener")}>
              <ExternalLink className="size-3.5" /> New Tab
            </Button>
          ) : null}
        </div>
      </div>
      {!target ? (
        <div className="mx-auto w-full max-w-[640px] pt-10">
          <Group footer="The shared accounts' dashboard needs the password that only the main machine and standbys keep. Switch on a provider under Sign-In to get this machine's own.">
            <Row icon={<IconTile icon={Server} color="gray" />} title={<span className="text-muted">No CLIProxyAPI dashboard here</span>} />
          </Group>
        </div>
      ) : mocked ? (
        <div className="flex flex-1 items-center justify-center rounded-xl bg-surface text-sm text-muted shadow-[0_0_0_0.5px_var(--border)]">
          CLIProxyAPI’s dashboard ({target}{remote ? `, on ${remote}` : ""}) fills this space.
        </div>
      ) : (
        <iframe
          key={src}
          title="CLIProxyAPI dashboard"
          src={src}
          className="min-h-0 w-full flex-1 rounded-xl bg-surface shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]"
        />
      )}
    </div>
  );
}
