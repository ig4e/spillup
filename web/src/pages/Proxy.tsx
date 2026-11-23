import { Button, Tabs } from "@heroui/react";
import { ExternalLink, Server } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { Group, IconTile, LargeTitle, Row } from "../components/ui";
import { api } from "../lib/api";
import { useQuery } from "../lib/hooks";

type Target = "shared" | "own";

const mocked = import.meta.env.VITE_MOCK === "1";

/** CLIProxyAPI's own dashboard, served through spillup so it opens signed in. */
export function ProxyPage() {
  const { data } = useOverview();
  const own = useQuery(api.own);
  const canShared = !!data?.can_manage;
  const canOwn = !!own.data?.families.length;
  const [picked, setPicked] = useState<Target | null>(null);
  const target: Target | null = picked ?? (canShared ? "shared" : canOwn ? "own" : null);

  return (
    <div className="flex h-[calc(100vh-6rem)] flex-col">
      <LargeTitle
        title="CLIProxyAPI"
        subtitle="Logins, providers, models, quotas and logs, straight from CLIProxyAPI. It opens signed in."
        actions={
          <>
            {canShared && canOwn ? (
              <Tabs selectedKey={target ?? "shared"} onSelectionChange={(k) => setPicked(String(k) as Target)} variant="primary">
                <Tabs.ListContainer>
                  <Tabs.List aria-label="Which CLIProxyAPI">
                    <Tabs.Tab id="shared" className="px-3 text-[13px] whitespace-nowrap">
                      Shared Accounts
                      <Tabs.Indicator />
                    </Tabs.Tab>
                    <Tabs.Tab id="own" className="px-3 text-[13px] whitespace-nowrap">
                      This Machine
                      <Tabs.Indicator />
                    </Tabs.Tab>
                  </Tabs.List>
                </Tabs.ListContainer>
              </Tabs>
            ) : null}
            {target ? (
              <Button size="sm" variant="secondary" onPress={() => window.open(`/cpa/${target}/`, "_blank", "noopener")}>
                <ExternalLink className="size-3.5" /> Open in New Tab
              </Button>
            ) : null}
          </>
        }
      />
      {!target ? (
        <Group footer="The shared accounts' dashboard needs the password that only the main machine and standbys keep. Turn on a separate sign-in under Accounts to get this machine's own.">
          <Row icon={<IconTile icon={Server} color="gray" />} title={<span className="text-muted">No CLIProxyAPI dashboard on this machine</span>} />
        </Group>
      ) : mocked ? (
        <div className="flex flex-1 items-center justify-center rounded-xl bg-surface text-sm text-muted shadow-[0_0_0_0.5px_var(--border)]">
          CLIProxyAPI’s dashboard for “{target}” appears here.
        </div>
      ) : (
        <iframe
          key={target}
          title="CLIProxyAPI dashboard"
          src={`/cpa/${target}/`}
          className="w-full flex-1 rounded-xl bg-surface shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]"
        />
      )}
    </div>
  );
}
