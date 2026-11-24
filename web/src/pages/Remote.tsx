import { Popover } from "@heroui/react";
import { ExternalLink, KeyRound, Link2, Monitor, MonitorSmartphone, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../components/Button";
import { TaskLog } from "../components/TaskProgress";
import { Dot, Group, IconTile, Row } from "../components/ui";
import { api } from "../lib/api";
import { useAction, useQuery } from "../lib/hooks";
import type { SetupProgress } from "../lib/types";

const mocked = import.meta.env.VITE_MOCK === "1";

function Status({ on, label }: { on: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1.5 rounded-full bg-surface px-2.5 py-1 text-xs text-muted">
      <Dot tone={on ? "success" : "muted"} /> {label}
    </span>
  );
}

/** Remote desktop to any of your machines, in the browser, through this machine's moonlight-web. */
export function RemotePage() {
  const remote = useQuery(api.remote, 5000);
  const machines = useQuery(api.machines, 15000);
  const { pending, run } = useAction();
  const [progress, setProgress] = useState<SetupProgress | null>(null);
  const st = remote.data;

  useEffect(() => {
    if (!progress?.running) return;
    const id = setInterval(async () => {
      const p = await api.setupProgress();
      setProgress(p);
      if (!p.running) void remote.reload();
    }, 1000);
    return () => clearInterval(id);
  }, [progress?.running, remote]);

  const setUp = async () => {
    await run("setup", api.remoteSetup);
    setProgress({ running: true, done: false, log: [] });
  };

  const ready = st?.web_running;
  const others = (machines.data?.machines ?? []).filter((m) => !m.is_self && m.online);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mb-3 flex flex-wrap items-center gap-2.5">
        <h1 className="text-[20px] font-bold tracking-tight">Remote</h1>
        {st ? (
          <>
            <Status on={st.sunshine_running} label={st.sunshine_running ? "Reachable (Sunshine)" : "Not reachable"} />
            <Status on={st.moonlight_installed} label="Moonlight app" />
            <Status on={st.web_running} label="Browser client" />
          </>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          {ready ? (
            <>
              <PinPopover machines={others.map((m) => m.name)} />
              <Button size="sm" variant="secondary" isPending={pending === "sync"} onPress={() => run("sync", api.remoteSync, remote.reload)}>
                {pending === "sync" ? null : <Link2 className="size-3.5" />} Pair All Machines
              </Button>
              <Button size="sm" variant="secondary" onPress={() => window.open("/remote/", "_blank", "noopener")}>
                <ExternalLink className="size-3.5" /> New Tab
              </Button>
            </>
          ) : null}
        </div>
      </div>

      {progress ? (
        <div className="mx-auto w-full max-w-[680px] pt-4">
          <TaskLog
            progress={progress}
            hint="Approve any administrator prompt. spillup installs Sunshine, Moonlight and the browser client, then pairs your machines."
            onRetry={() => setProgress(null)}
            onDone={() => {
              setProgress(null);
              void remote.reload();
            }}
          />
        </div>
      ) : !st ? null : !ready ? (
        <div className="mx-auto w-full max-w-[680px] pt-4">
          <Group
            title="Remote Desktop"
            footer="Installs Sunshine so your other machines can reach this one, Moonlight as the app, and a browser client so you can reach any machine right here. spillup pairs them over Tailscale: no PINs to type."
          >
            <Row icon={<IconTile icon={Sun} color="orange" />} title="Sunshine" subtitle="Lets your other machines see and control this one" value={st.sunshine_installed ? (st.sunshine_running ? "Running" : "Installed") : "Not installed"} />
            <Row icon={<IconTile icon={Monitor} color="indigo" />} title="Moonlight" subtitle="The app, for the lowest latency" value={st.moonlight_installed ? "Installed" : "Not installed"} />
            <Row icon={<IconTile icon={MonitorSmartphone} color="teal" />} title="Browser client" subtitle="Reach any machine from this dashboard" value={st.web_installed ? "Starting…" : "Not installed"} />
            <Row icon={<IconTile icon={Link2} color="blue" />} title={pending === "setup" ? "Setting Up…" : "Set Up Remote Desktop"} tone="accent" busy={pending === "setup"} onPress={pending ? undefined : setUp} />
          </Group>
        </div>
      ) : mocked ? (
        <div className="flex flex-1 items-center justify-center rounded-xl bg-surface text-sm text-muted">Your machines' desktops stream here.</div>
      ) : (
        <iframe
          title="Remote desktop"
          src="/remote/"
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
