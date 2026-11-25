import { Clock, FolderGit2, Heart, KeyRound, Lock, Network, Palette, Server, Smartphone, SquareTerminal, Undo2 } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { SetupPreview } from "../components/SetupPreview";
import { QR } from "../components/QR";
import { BrandTile } from "../components/brand";
import { CopyButton, Group, IconTile, LargeTitle, Mono, Row, Segmented, Toggle, holdName, osName } from "../components/ui";
import { api } from "../lib/api";
import { useAction, useRemote, useTheme } from "../lib/hooks";
import type { Keys } from "../lib/types";

export function SettingsPage() {
  const { data } = useOverview();
  const [theme, setTheme] = useTheme();
  const remote = useRemote();
  return (
    <>
      <LargeTitle title="Settings" />

      <Group title="Appearance">
        <Row
          icon={<IconTile icon={Palette} color="blue" />}
          title="Appearance"
          accessory={
            <Segmented
              label="Appearance"
              value={theme}
              onChange={setTheme}
              options={[
                { id: "system", label: "Auto" },
                { id: "light", label: "Light" },
                { id: "dark", label: "Dark" },
              ]}
            />
          }
        />
      </Group>

      {remote ? null : <DevicesGroup />}
      {remote ? null : <KeysGroup />}

      <Group
        title="Accounts Service"
        footer={
          <>
            Choose the main machine on the Machines page, and where the logins live on the Accounts page.
          </>
        }
      >
        <Row icon={<IconTile icon={Server} color="green" />} title="Role" value={data ? holdName[data.hold] : "—"} to="/machines" />
        <Row icon={<IconTile icon={Network} color="teal" />} title="Address" value={data?.router_url ? <Mono copy>{data.router_url}</Mono> : "Unavailable"} />
        <Row icon={<IconTile icon={Clock} color="orange" />} title="Fail over after" value={data ? `${Math.round(data.settings.failover_after_seconds / 60)} minutes offline` : "—"} />
        <Row icon={<IconTile icon={Undo2} color="indigo" />} title="Hand back to the main machine" value={data?.settings.auto_failback ? "Automatically" : "Manually"} />
      </Group>

      {remote || !data?.installed ? null : <SetupPreview />}

      <Group title="About">
        <Row icon={<img src="/icon.svg" alt="" className="size-7 rounded-[7px]" />} title="spillup" value={data ? `${data.version} · ${osName(data.os)}` : "—"} />
        <Row
          icon={<BrandTile brand="github" icon={FolderGit2} />}
          title="Source"
          tone="accent"
          value="ig4e/spillup"
          onPress={() => window.open("https://github.com/ig4e/spillup", "_blank", "noopener")}
        />
        <Row
          icon={<IconTile icon={Heart} color="pink" />}
          title="Built on spinup"
          value="darkyeg/spinup"
          onPress={() => window.open("https://github.com/darkyeg/spinup", "_blank", "noopener")}
        />
      </Group>
    </>
  );
}

function KeysGroup() {
  const [keys, setKeys] = useState<Keys | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const reveal = async () => {
    setLoading(true);
    try {
      setKeys(await api.keys());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const secret = (value?: string) =>
    value ? (
      <span className="flex items-center gap-0.5">
        <span className="max-w-52 truncate font-mono text-[12px] text-muted">{value}</span>
        <CopyButton value={value} />
      </span>
    ) : (
      <span className="font-mono text-muted">••••••••••••</span>
    );

  return (
    <Group
      title="Keys"
      footer={error ?? "A new machine asks for the API key during setup; a new standby also asks for the dashboard password. Keep both private."}
    >
      <Row icon={<IconTile icon={KeyRound} color="yellow" />} title="API key" accessory={secret(keys?.api_key)} />
      <Row
        icon={<IconTile icon={Lock} color="red" />}
        title="Dashboard password"
        subtitle={keys && !keys.management_password ? "Kept only on the main machine and standbys" : undefined}
        accessory={keys && !keys.management_password ? undefined : secret(keys?.management_password)}
      />
      {keys ? (
        <Row title="Hide Keys" tone="accent" onPress={() => setKeys(null)} />
      ) : (
        <Row title={loading ? "Loading…" : "Show Keys"} tone="accent" onPress={reveal} />
      )}
    </Group>
  );
}

/** Your phone and other devices: the dashboard over Tailscale, and terminals on this machine. */
function DevicesGroup() {
  const { data, reload } = useOverview();
  const { pending, run } = useAction();
  const [devices, setDevices] = useState<boolean | null>(null);
  const [terminals, setTerminals] = useState<boolean | null>(null);
  const s = data?.settings;
  const deviceOn = devices ?? s?.device_access ?? true;
  const terminalsOn = terminals ?? s?.terminals ?? true;
  const save = async (d: boolean, t: boolean) => {
    setDevices(d);
    setTerminals(t);
    if (!(await run("access", () => api.setAccess(d, t), reload))) {
      setDevices(null);
      setTerminals(null);
    }
  };
  return (
    <Group
      title="Your Devices"
      footer="Any device signed in to your Tailscale account (your phone, a tablet, another computer) opens this dashboard at this address, at home or away. Install Tailscale on it and sign in with the same account. Other people's devices and your local network can't reach it, and keys and sign-ins stay on this machine."
    >
      {deviceOn && s?.device_url ? (
        <div className="flex flex-col items-center gap-4 px-4 py-4 sm:flex-row sm:items-center">
          <QR value={s.device_url} size={148} />
          <div className="min-w-0 flex-1 text-center sm:text-left">
            <div className="text-[15px] font-semibold">Open on Your Phone</div>
            <div className="mt-1 text-[13px] text-muted">Point your phone’s camera here, then Share → Add to Home Screen for an app icon.</div>
            <div className="mt-2.5 flex justify-center sm:justify-start">
              <Mono copy>{s.device_url}</Mono>
            </div>
          </div>
        </div>
      ) : null}
      <Row
        icon={<IconTile icon={Smartphone} color="blue" />}
        title="Open from your devices"
        subtitle="Over Tailscale only"
        accessory={<Toggle label="Open from your devices" isDisabled={pending === "access"} isSelected={deviceOn} onChange={(on) => void save(on, terminalsOn)} />}
      />
      <Row
        icon={<IconTile icon={SquareTerminal} color="graphite" />}
        title="Terminals on this machine"
        subtitle="From this dashboard, here or on your devices"
        accessory={<Toggle label="Terminals on this machine" isDisabled={pending === "access"} isSelected={terminalsOn} onChange={(on) => void save(deviceOn, on)} />}
      />
    </Group>
  );
}
