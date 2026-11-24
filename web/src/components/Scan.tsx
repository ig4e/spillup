import { Button } from "./Button";
import { Radar } from "lucide-react";
import { useState } from "react";
import { api } from "../lib/api";
import type { Device } from "../lib/types";
import { Dot, Group, IconTile, Row, holdName, osName, osTile } from "./ui";

/** Finds which devices on the tailnet run spillup, and which of yours still need it. */
export function Scan() {
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [scanning, setScanning] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const scan = async () => {
    setScanning(true);
    try {
      setDevices(await api.scan());
      setProblem(null);
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setScanning(false);
    }
  };

  const missing = devices?.filter((d) => d.yours && d.online && !d.spillup) ?? [];
  return (
    <Group
      title="Find spillup Devices"
      footer={
        problem ??
        (devices
          ? missing.length
            ? `${missing.length} of your devices ${missing.length === 1 ? "doesn't" : "don't"} run spillup yet. Install it there: devices on your Tailscale account join by themselves.`
            : "Every device of yours that's online runs spillup, or is a phone or tablet that uses the main machine directly."
          : "Looks for spillup on every device on your Tailscale network, at home or away.")
      }
    >
      <Row
        icon={<IconTile icon={Radar} color="teal" />}
        title="Scan Your Tailnet"
        subtitle={devices ? `${devices.filter((d) => d.spillup).length} of ${devices.length} devices run spillup` : undefined}
        accessory={
          <Button size="sm" variant="secondary" isPending={scanning} onPress={scan}>
            <>{devices ? "Scan Again" : "Scan"}</>
          </Button>
        }
      />
      {devices?.map((d) => {
        const t = osTile(d.os);
        return (
          <Row
            key={d.name}
            icon={<IconTile icon={t.icon} color={d.online ? t.color : "gray"} />}
            title={
              <span className="flex items-center gap-2">
                {d.name}
                {!d.yours ? <span className="text-xs text-muted">Shared with you</span> : null}
              </span>
            }
            subtitle={`${osName(d.os)} · ${d.ip}`}
            value={
              <span className="flex items-center gap-1.5 text-[13px]">
                <Dot tone={!d.online ? "muted" : d.spillup ? "success" : "warning"} />
                {!d.online ? "Offline" : d.spillup ? `spillup · ${d.hold ? holdName[d.hold] : ""}` : "No spillup"}
              </span>
            }
          />
        );
      })}
    </Group>
  );
}
