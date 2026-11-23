import { Tabs } from "@heroui/react";
import { Clock, FolderGit2, Heart, KeyRound, Lock, Network, Palette, Server, Undo2 } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { CopyButton, Group, IconTile, LargeTitle, Mono, Row, holdName, osName } from "../components/ui";
import { api } from "../lib/api";
import { useTheme, type Theme } from "../lib/hooks";
import type { Keys } from "../lib/types";

export function SettingsPage() {
  const { data } = useOverview();
  const [theme, setTheme] = useTheme();
  return (
    <>
      <LargeTitle title="Settings" />

      <Group title="Appearance">
        <Row
          icon={<IconTile icon={Palette} color="blue" />}
          title="Appearance"
          accessory={
            <Tabs selectedKey={theme} onSelectionChange={(k) => setTheme(String(k) as Theme)} variant="primary">
              <Tabs.ListContainer>
                <Tabs.List aria-label="Appearance">
                  {(["system", "light", "dark"] as const).map((t) => (
                    <Tabs.Tab key={t} id={t} className="px-3 text-[13px]">
                      {t === "system" ? "Auto" : t === "light" ? "Light" : "Dark"}
                      <Tabs.Indicator />
                    </Tabs.Tab>
                  ))}
                </Tabs.List>
              </Tabs.ListContainer>
            </Tabs>
          }
        />
      </Group>

      <KeysGroup />

      <Group
        title="Accounts Service"
        footer={
          <>
            Stored in spinup’s config.json; run <Mono>spinup setup</Mono> again after editing it. Choose the main machine on the Machines page.
          </>
        }
      >
        <Row icon={<IconTile icon={Server} color="green" />} title="Role" value={data ? holdName[data.hold] : "—"} to="/machines" />
        <Row icon={<IconTile icon={Network} color="teal" />} title="Address" accessory={<Mono copy>{data?.router_url ?? "—"}</Mono>} />
        <Row icon={<IconTile icon={Clock} color="orange" />} title="Fail over after" value={data ? `${Math.round(data.settings.failover_after_seconds / 60)} minutes offline` : "—"} />
        <Row icon={<IconTile icon={Undo2} color="indigo" />} title="Hand back to the main machine" value={data?.settings.auto_failback ? "Automatically" : "Manually"} />
      </Group>

      <Group title="About">
        <Row icon={<img src="/icon.svg" alt="" className="size-7 rounded-[7px]" />} title="spillup" value={data ? `${data.version} · ${osName(data.os)}` : "—"} />
        <Row
          icon={<IconTile icon={FolderGit2} color="graphite" />}
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
