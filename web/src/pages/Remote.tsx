import { Popover } from "@heroui/react";
import { ChevronRight, KeyRound, Link2, MonitorSmartphone, Play } from "lucide-react";
import { useState } from "react";
import { Button } from "../components/Button";
import { TaskLog } from "../components/TaskProgress";
import { Group, GroupSkeleton, IconTile, LargeTitle, Row, osName } from "../components/ui";
import { RemoteStream } from "../components/RemoteStream";
import { OsTile } from "../components/brand";
import { api } from "../lib/api";
import { useAction, useQuery, useTaskProgress } from "../lib/hooks";
import type { Machine, RemoteHost } from "../lib/types";

interface Streaming {
  host: RemoteHost;
  machine: string;
  appId: number;
  title: string;
}

/** Remote desktop to any of your machines: pick one and an app, and it streams right here. */
export function RemotePage() {
  const [streaming, setStreaming] = useState<Streaming | null>(null);
  if (streaming) return <RemoteStream hostId={streaming.host.id} appId={streaming.appId} machine={streaming.machine} title={streaming.title} onEnd={() => setStreaming(null)} />;
  return <RemoteBrowser onStart={setStreaming} />;
}

function RemoteBrowser({ onStart }: { onStart: (stream: Streaming) => void }) {
  const remote = useQuery(api.remote, 5000);
  const machines = useQuery(api.localMachines, 15000);
  const { pending, run } = useAction();
  const { progress, setProgress, connectionError } = useTaskProgress("remote");
  const st = remote.data;
  const all = machines.data?.machines ?? [];
  const selectedHost = new URLSearchParams(window.location.search).get("hostId");

  const setUp = async () => {
    if (await run("setup", api.remoteSetup)) setProgress({ running: true, done: false, log: [] });
  };

  const ready = st?.web_running;
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <LargeTitle bleed title="Remote" />
      <div className="mx-auto w-full min-w-0 max-w-[760px] overflow-y-auto px-1 pb-4">
        {progress ? (
          <TaskLog
            progress={progress}
            connectionError={connectionError}
            hint="Approve the administrator prompt on this machine."
            onRetry={() => setProgress(null)}
            onDone={() => {
              setProgress(null);
              void remote.reload();
            }}
          />
        ) : !st ? (
          remote.error ? <p role="alert" className="break-words text-sm text-muted">{remote.error.message}</p> : <GroupSkeleton rows={2} />
        ) : !ready ? (
          <Group footer={st.problem || "Install Sunshine and Moonlight to connect over your Tailnet."}>
            <Row icon={<IconTile icon={MonitorSmartphone} color="teal" />} title={pending === "setup" ? "Setting up…" : st.web_installed ? "Repair remote desktop" : "Set up remote desktop"} busy={pending === "setup"} tone="accent" onPress={pending ? undefined : setUp} />
          </Group>
        ) : st.hosts.length === 0 ? (
          <Group footer="Set up remote desktop on the other machine, then pair it here.">
            <Row icon={<IconTile icon={MonitorSmartphone} color="gray" />} title="No paired machines" />
          </Group>
        ) : (
          <div className="grid min-w-0 grid-cols-1 items-start gap-3 sm:grid-cols-2">
            {[...st.hosts].sort((a, b) => Number(String(b.id) === selectedHost) - Number(String(a.id) === selectedHost) || (a.machine || a.name).localeCompare(b.machine || b.name)).map((h) => (
              <HostCard
                key={h.id}
                host={h}
                machine={all.find((m) => m.name === h.machine)}
                pairing={pending === "sync"}
                onPair={() => run("sync", api.remoteSync, remote.reload)}
                onStart={(appId, title) => onStart({ host: h, machine: h.machine || h.name, appId, title })}
              />
            ))}
          </div>
        )}
        {ready && !progress && remote.refreshError ? (
          <Group footer={remote.refreshError.message}>
            <Row title="Showing the last machine list" value="Retry" onPress={() => void remote.reload()} />
            <Row title="Repair remote desktop" tone="accent" busy={pending === "setup"} onPress={pending ? undefined : setUp} />
          </Group>
        ) : null}
        {ready && !progress ? (
          <div className="mt-5 flex min-w-0 flex-wrap items-center gap-2">
            <Button size="sm" variant="secondary" isPending={pending === "sync"} onPress={() => run("sync", api.remoteSync, remote.reload)}>
              <Link2 className="size-4" /> Pair machines
            </Button>
            <PinPopover machines={all.filter((m) => !m.is_self && m.online).map((m) => m.name)} />
          </div>
        ) : null}
      </div>
    </div>
  );
}

function HostCard({
  host,
  machine,
  pairing,
  onPair,
  onStart,
}: {
  host: RemoteHost;
  machine?: Machine;
  pairing: boolean;
  onPair: () => void;
  onStart: (appId: number, title: string) => void;
}) {
  const online = machine?.online ?? host.state !== "Offline";
  return (
    <section className="group-list min-w-0 rounded-2xl bg-surface p-1">
      <div className="flex min-w-0 items-center gap-3 px-3 py-3">
        <OsTile os={machine?.os} dim={!online} size="lg" />
        <div className="min-w-0 flex-1">
          <h2 className="break-words text-[16px] font-semibold leading-tight">{host.machine || host.name}</h2>
          <div className="mt-0.5 text-[13px] text-muted">{!online ? "Offline" : !host.paired ? "Pairing required" : osName(machine?.os)}</div>
        </div>
      </div>
      {!host.paired ? (
        <Button className="m-2" size="sm" variant="secondary" isDisabled={!online} isPending={pairing} onPress={onPair}>Pair</Button>
      ) : online ? <HostApps host={host} onStart={onStart} /> : null}
    </section>
  );
}

function HostApps({ host, onStart }: { host: RemoteHost; onStart: (id: number, title: string) => void }) {
  const apps = useQuery(() => api.remoteApps(host.id), 15000, JSON.stringify(["remoteApps", host.id, host.machine || host.name]));
  const list = [...(apps.data ?? [])].sort((a, b) => Number(b.title.toLowerCase() === "desktop") - Number(a.title.toLowerCase() === "desktop") || a.title.localeCompare(b.title));
  return list.length ? (
    <div className="border-t border-border/70 px-1">
      {list.map((app) => (
        <button key={app.app_id} type="button" onClick={() => onStart(app.app_id, app.title)}
          className="flex min-h-11 w-full min-w-0 items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[14px] hover:bg-surface-hover focus-visible:outline-accent">
          <Play aria-hidden className="size-3.5 shrink-0 text-accent" />
          <span className="min-w-0 flex-1 break-words">{app.title}</span>
          <ChevronRight aria-hidden className="size-4 shrink-0 text-muted" />
        </button>
      ))}
    </div>
  ) : (
    <p className="break-words px-3 pb-3 text-xs text-muted">{apps.error ? "Couldn’t read the app list. Retrying…" : apps.data ? "No apps published by Sunshine." : "Loading apps…"}</p>
  );
}

/** For Moonlight, the app: enter the PIN it shows, and the machine it's pairing with confirms it. */
function PinPopover({ machines }: { machines: string[] }) {
  const [pin, setPin] = useState("");
  const [machine, setMachine] = useState<string>("");
  const { pending, run } = useAction();
  return (
    <Popover>
      <Button size="sm" variant="secondary">
        <KeyRound className="size-3.5" /> Moonlight PIN
      </Button>
      <Popover.Content placement="bottom end" className="w-72 max-w-[calc(100vw-2rem)]">
        <Popover.Dialog className="flex flex-col gap-3 p-3.5">
          <div className="text-[13px] font-semibold">Pair the Moonlight app</div>
          <p className="text-xs text-muted">Enter the four-digit PIN from Moonlight.</p>
          <select
            value={machine}
            onChange={(e) => setMachine(e.target.value)}
            className="rounded-lg bg-surface px-2.5 py-1.5 text-[13px] outline-none"
            aria-label="The machine Moonlight is pairing with"
          >
            <option value="">This machine</option>
            {machines.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <input
            inputMode="numeric"
            maxLength={4}
            placeholder="1234"
            value={pin}
            onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))}
            aria-label="Moonlight's PIN"
            className="rounded-lg bg-surface px-2.5 py-1.5 text-center font-mono text-[18px] tracking-[0.4em] outline-none focus:ring-2 focus:ring-accent/50"
          />
          <Button size="sm" isDisabled={pin.length !== 4} isPending={pending === "pin"} onPress={() => run("pin", () => api.sunshinePin(machine || null, pin), () => setPin(""))}>
            Pair
          </Button>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
