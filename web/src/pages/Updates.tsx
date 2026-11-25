import { Button } from "../components/Button";
import { CloudDownload, Package, RefreshCw, RotateCcw } from "lucide-react";
import { useState } from "react";
import { TaskLog } from "../components/TaskProgress";
import { Group, GroupSkeleton, IconTile, Row, Toggle } from "../components/ui";
import type { FleetPreview } from "../lib/types";
import { useOptimisticSetting } from "../lib/useOptimisticSetting";
import { api, localDashboard } from "../lib/api";
import { useAction, useQuery, useRemote, useTaskProgress } from "../lib/hooks";

export function UpdatesSection() {
  const update = useQuery(api.update);
  const { pending, run } = useAction();
  const auto = useOptimisticSetting(api.update, update.data, api.setAutoUpdate, (previous, enabled) => ({ ...previous, auto: enabled }));
  const remote = useRemote();
  const data = auto.data;
  const newer = data?.latest && data.latest.replace(/^v/, "") !== data.current.replace(/^v/, "");

  return (
    <>
      {update.error ? <Group>
        <Row title="Couldn’t check for updates" subtitle={update.error.message} />
        <Row title="Try Again" tone="accent" onPress={() => void update.reload()} />
      </Group> : null}
      {!data && update.error ? null : !data ? (
        <GroupSkeleton rows={2} />
      ) : (
        <>
          <div className="mb-7 flex flex-col items-center rounded-2xl bg-surface px-6 py-9 text-center shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
            <IconTile icon={newer ? CloudDownload : RefreshCw} color={newer ? "blue" : "green"} size="lg" />
            <div className="mt-3 text-[22px] font-semibold tracking-tight">{!data.latest ? "Release check unavailable" : newer ? `spillup ${data.latest} Is Available` : "spillup Is Up to Date"}</div>
            <div className="mt-1 text-[14px] text-muted">This machine runs {data.current}.</div>
            {newer ? (
              <Button className="mt-5" isPending={pending === "update"} onPress={() => run("update", api.startUpdate)}>
                <>Update Now</>
              </Button>
            ) : null}
          </div>
          <Group footer="For a staged rollout, turn off automatic updates on participating machines.">
            <Row
              icon={<IconTile icon={CloudDownload} color="blue" />}
              title="Update automatically"
              accessory={
                <Toggle
                  label="Update automatically"
                  isSelected={data.auto}
                  isDisabled={auto.pending}
                  onChange={auto.save}
                />
              }
            />
          </Group>
          {remote ? null : <FleetRollout />}
          {data.system_managed && localDashboard && !remote ? <SystemUpdates pending={data.system ?? []} reload={update.reload} /> : null}
          <Group footer="Restarts Spillup and this dashboard. On a sole account holder, shared requests may pause until the proxy returns.">
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
  const { progress, setProgress, connectionError } = useTaskProgress("update");
  const { pending: busy, run } = useAction();
  if (progress) {
    return (
      <TaskLog
        progress={progress}
        connectionError={connectionError}
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

function FleetRollout() {
  const [plan, setPlan] = useState<FleetPreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const preview = async () => {
    setBusy(true);
    setError("");
    setPlan(null);
    try {
      setPlan(await api.rolloutPlan());
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };
  if (!localDashboard) return null;

  return (
    <Group title="Staged Fleet Update">
      <Row title={busy ? "Checking machines…" : "Preview rollout order"} tone="accent" busy={busy} onPress={preview} />
      {plan?.output ? <pre className="overflow-x-auto whitespace-pre-wrap break-words px-4 py-3 text-xs">{plan.output}</pre> : null}
      {error || plan?.problem ? <p role="alert" className="px-4 py-2 text-sm text-warning">{error || plan?.problem}</p> : null}
    </Group>
  );
}
