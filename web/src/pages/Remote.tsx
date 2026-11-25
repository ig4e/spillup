import { Popover } from "@heroui/react";
import { Expand, KeyRound, Link2, Monitor, MonitorSmartphone, Play, Sun, X } from "lucide-react";
import { useRef, useState } from "react";
import { Button } from "../components/Button";
import { TaskLog } from "../components/TaskProgress";
import { Dot, Group, IconTile, LargeTitle, Row, osName } from "../components/ui";
import { OsTile } from "../components/brand";
import { api } from "../lib/api";
import { useAction, useQuery, useTaskProgress } from "../lib/hooks";
import type { Machine, RemoteHost } from "../lib/types";

const mocked = import.meta.env.VITE_MOCK === "1";

interface Streaming {
  host: RemoteHost;
  machine: string;
  appId: number;
  title: string;
}

/** Remote desktop to any of your machines: pick one and an app, and it streams right here. */
export function RemotePage() {
  const remote = useQuery(api.remote, 5000);
  const machines = useQuery(api.machines, 15000);
  const { pending, run } = useAction();
  const { progress, setProgress, connectionError } = useTaskProgress("remote");
  const [streaming, setStreaming] = useState<Streaming | null>(null);
  const st = remote.data;
  const all = machines.data?.machines ?? [];


  const setUp = async () => {
    if (await run("setup", api.remoteSetup)) setProgress({ running: true, done: false, log: [] });
  };

  if (streaming) return <Stream s={streaming} onEnd={() => setStreaming(null)} />;

  const ready = st?.web_running;
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <LargeTitle
        bleed
        title="Remote"
        actions={
          ready ? (
            <>
              <PinPopover machines={all.filter((m) => !m.is_self && m.online).map((m) => m.name)} />
              <Button size="sm" variant="secondary" isPending={pending === "sync"} onPress={() => run("sync", api.remoteSync, remote.reload)}>
                {pending === "sync" ? null : <Link2 className="size-3.5" />} Pair All Machines
              </Button>
            </>
          ) : undefined
        }
      />
      <div className="mx-auto w-full max-w-[920px] px-1">
        {st ? (
          <div className="mb-5 flex flex-wrap gap-2">
            <Chip on={st.sunshine_running} label={st.sunshine_running ? "Others can reach this machine" : "Others can't reach this machine"} />
            <Chip on={st.moonlight_installed} label="Moonlight app" />
            <Chip on={!!st.web_running} label="Browser client" />
          </div>
        ) : null}
        {progress ? (
          <TaskLog
            progress={progress}
          connectionError={connectionError}
            hint="Approve any administrator prompt. spillup installs Sunshine, Moonlight and the browser client, then pairs your machines."
            onRetry={() => setProgress(null)}
            onDone={() => {
              setProgress(null);
              void remote.reload();
            }}
          />
        ) : !st ? null : !ready ? (
          <Group
            title="Remote Desktop"
            footer="Installs Sunshine so your other machines can reach this one, Moonlight as the app, and the browser client so you can reach any machine right here. spillup pairs them over Tailscale: no PINs to type."
          >
            <Row icon={<IconTile icon={Sun} color="orange" />} title="Sunshine" subtitle="Lets your other machines see and control this one" value={st.sunshine_installed ? (st.sunshine_running ? "Running" : "Installed") : "Not installed"} />
            <Row icon={<IconTile icon={Monitor} color="indigo" />} title="Moonlight" subtitle="The app, for the lowest latency" value={st.moonlight_installed ? "Installed" : "Not installed"} />
            <Row icon={<IconTile icon={MonitorSmartphone} color="teal" />} title="Browser client" subtitle="Reach any machine from this dashboard" value={st.web_installed ? "Starting…" : "Not installed"} />
            <Row icon={<IconTile icon={Link2} color="blue" />} title={pending === "setup" ? "Setting Up…" : "Set Up Remote Desktop"} tone="accent" busy={pending === "setup"} onPress={pending ? undefined : setUp} />
          </Group>
        ) : st.hosts.length === 0 ? (
          <Group footer="Set remote desktop up on your other machines too; they appear here within a few minutes, or press Pair All Machines.">
            <Row icon={<IconTile icon={MonitorSmartphone} color="gray" />} title={<span className="text-muted">No machines to reach yet</span>} />
          </Group>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {st.hosts.map((h) => (
              <HostCard
                key={h.id}
                host={h}
                machine={all.find((m) => m.name === h.machine)}
                pairing={pending === "sync"}
                onPair={() => run("sync", api.remoteSync, remote.reload)}
                onStart={(appId, title) => setStreaming({ host: h, machine: h.machine || h.name, appId, title })}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function Chip({ on, label }: { on: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1.5 rounded-full bg-surface px-2.5 py-1 text-xs text-muted">
      <Dot tone={on ? "success" : "muted"} /> {label}
    </span>
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
  const apps = useQuery(() => (host.paired ? api.remoteApps(host.id) : Promise.resolve([])), 0);
  const online = machine?.online ?? host.state !== "Offline";
  const list = apps.data ?? [];
  const desktop = list.find((a) => a.title.toLowerCase() === "desktop") ?? list[0];
  return (
    <div className="flex flex-col gap-4 rounded-xl bg-surface p-4">
      <div className="flex items-center gap-3">
        <OsTile os={machine?.os} dim={!online} size="lg" />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[15px] font-semibold">{host.machine || host.name}</div>
          <div className="flex items-center gap-1.5 text-xs text-muted">
            <Dot tone={!online ? "muted" : host.paired ? "success" : "warning"} className="size-1.5" />
            {!online ? "Offline" : host.paired ? `Ready · ${osName(machine?.os)}` : "Not paired yet"}
          </div>
        </div>
      </div>
      {!host.paired ? (
        <Button size="sm" variant="secondary" isPending={pairing} onPress={onPair}>
          Pair
        </Button>
      ) : (
        <div className="flex flex-wrap gap-2">
          {desktop ? (
            <Button size="sm" isDisabled={!online} onPress={() => onStart(desktop.app_id, desktop.title)}>
              <Play className="size-3.5" /> {desktop.title}
            </Button>
          ) : (
            <span className="text-xs text-muted">{apps.error ? apps.error.message : "Reading apps…"}</span>
          )}
          {list
            .filter((a) => a !== desktop)
            .map((a) => (
              <Button key={a.app_id} size="sm" variant="secondary" isDisabled={!online} onPress={() => onStart(a.app_id, a.title)}>
                {a.title}
              </Button>
            ))}
        </div>
      )}
    </div>
  );
}

/** A stream, as big as the page, with a slim bar to go full screen or end it. */
function Stream({ s, onEnd }: { s: Streaming; onEnd: () => void }) {
  const frame = useRef<HTMLIFrameElement>(null);
  const src = `/remote/stream.html?hostId=${s.host.id}&appId=${s.appId}`;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <LargeTitle
        bleed
        title={`${s.machine} · ${s.title}`}
        actions={
          <>
            <Button size="sm" variant="secondary" onPress={() => void frame.current?.requestFullscreen()}>
              <Expand className="size-3.5" /> Full Screen
            </Button>
            <Button size="sm" variant="danger-soft" onPress={onEnd}>
              <X className="size-3.5" /> End
            </Button>
          </>
        }
      />
      {mocked ? (
        <div className="flex flex-1 items-center justify-center rounded-xl bg-black text-sm text-white/60">{s.machine}'s screen streams here.</div>
      ) : (
        <iframe
          ref={frame}
          title={`${s.machine} remote desktop`}
          src={src}
          allow="fullscreen; gamepad; keyboard-map; clipboard-read; clipboard-write; autoplay"
          allowFullScreen
          className="min-h-0 w-full flex-1 rounded-xl bg-black"
        />
      )}
    </div>
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
      <Popover.Content placement="bottom end" className="w-72">
        <Popover.Dialog className="flex flex-col gap-3 p-3.5">
          <div className="text-[13px] font-semibold">Pair the Moonlight app</div>
          <p className="text-xs text-muted">In Moonlight, add the machine by its name and pick it: it shows a PIN. Enter it here.</p>
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
