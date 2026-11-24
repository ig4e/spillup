import { Button } from "../components/Button";
import { CloudDownload, Package, RefreshCw, RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import { TaskLog } from "../components/TaskProgress";
import type { SetupProgress } from "../lib/types";
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
                <>Update Now</>
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
          {data.system_managed ? <SystemUpdates pending={data.system ?? []} reload={update.reload} /> : null}
          <Group footer="Restarts spillup's background service and this dashboard. Running requests finish and the accounts move on first, so nothing is cut off.">
            <Row
              icon={<IconTile icon={RotateCcw} color="gray" />}
              title={pending === "restart" ? "Restarting…" : "Restart spillup"}
              tone="accent"
              busy={pending === "restart"}
              onPress={pending ? undefined : () => run("restart", api.restart)}
            />
          </Group>
        </>
      )}
    </>
  );
}

/** The system's own packages, which own Tailscale, Sunshine and Moonlight here: a full update you start. */
function SystemUpdates({ pending, reload }: { pending: string[]; reload: () => Promise<void> }) {
  const [progress, setProgress] = useState<SetupProgress | null>(null);
  const { pending: busy, run } = useAction();
  useEffect(() => {
    if (!progress?.running) return;
    const id = setInterval(async () => setProgress(await api.setupProgress()), 1000);
    return () => clearInterval(id);
  }, [progress?.running]);
  if (progress) {
    return (
      <TaskLog
        progress={progress}
        hint="spillup runs a full system update (pacman -Syu), the only kind Arch supports."
        onRetry={() => setProgress(null)}
        onDone={() => {
          setProgress(null);
          void reload();
        }}
      />
    );
  }
  const ours = pending.filter((p) => ["tailscale", "sunshine", "moonlight-qt"].includes(p));
  return (
    <Group
      title="System Packages"
      footer={
        pending.length
          ? `Tailscale, Sunshine and Moonlight come with the system here, and Arch updates only the whole system: this runs pacman -Syu and asks for your password.`
          : "Tailscale, Sunshine and Moonlight come with the system here; they're up to date."
      }
    >
      <Row
        icon={<IconTile icon={Package} color={ours.length ? "orange" : "gray"} />}
        title={pending.length ? `${pending.length} package${pending.length === 1 ? "" : "s"} can update` : "Up to date"}
        subtitle={ours.length ? `Including ${ours.join(", ")}` : undefined}
      />
      {pending.length ? (
        <Row
          icon={<IconTile icon={CloudDownload} color="blue" />}
          title={busy === "system" ? "Starting…" : "Update System"}
          tone="accent"
          busy={busy === "system"}
          onPress={busy ? undefined : async () => {
            if (await run("system", api.updateSystem)) setProgress({ running: true, done: false, log: [] });
          }}
        />
      ) : null}
    </Group>
  );
}
