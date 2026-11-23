import { Button, Spinner } from "@heroui/react";
import { CloudDownload, RefreshCw } from "lucide-react";
import { useState } from "react";
import { Group, GroupSkeleton, IconTile, LargeTitle, Row, Toggle } from "../components/ui";
import { api } from "../lib/api";
import { useAction, useQuery } from "../lib/hooks";

export function UpdatesPage() {
  const update = useQuery(api.update);
  const { pending, run } = useAction();
  const [auto, setAuto] = useState<boolean | null>(null);
  const data = update.data;
  const newer = data?.latest && data.latest.replace(/^v/, "") !== data.current.replace(/^v/, "");

  return (
    <>
      <LargeTitle title="Updates" subtitle="Keep spillup, CLIProxyAPI, your skills and Tailscale current on this machine." />
      {!data ? (
        <GroupSkeleton rows={2} />
      ) : (
        <>
          <div className="mb-7 flex flex-col items-center rounded-2xl bg-surface px-6 py-9 text-center shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
            <IconTile icon={newer ? CloudDownload : RefreshCw} color={newer ? "blue" : "green"} size="lg" />
            <div className="mt-3 text-[22px] font-semibold tracking-tight">{newer ? `spillup ${data.latest} Is Available` : "spillup Is Up to Date"}</div>
            <div className="mt-1 text-[14px] text-muted">This machine runs {data.current}.</div>
            {newer ? (
              <Button className="mt-5" isPending={pending === "update"} onPress={() => run("update", api.startUpdate)}>
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner size="sm" color="current" /> : null}
                    Update Now
                  </>
                )}
              </Button>
            ) : null}
          </div>
          <Group footer="The service checks every few hours and installs new releases by itself, handing the accounts on first so nothing running is cut off.">
            <Row
              icon={<IconTile icon={CloudDownload} color="blue" />}
              title="Update automatically"
              accessory={
                <Toggle
                  label="Update automatically"
                  isSelected={auto ?? data.auto}
                  onChange={async (on) => {
                    setAuto(on);
                    if (!(await run("auto", () => api.setAutoUpdate(on), update.reload))) setAuto(null);
                  }}
                />
              }
            />
          </Group>
        </>
      )}
    </>
  );
}
