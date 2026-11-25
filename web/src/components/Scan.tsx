import { Button } from "./Button";
import { Radar } from "lucide-react";
import { useState } from "react";
import { api } from "../lib/api";
import type { Device } from "../lib/types";
import { Dot, Group, IconTile, Row, holdName, osName } from "./ui";
import { OsTile } from "./brand";

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

  return (
    <Group
      title="Tailnet discovery"
      footer={problem ?? "Checks which online Tailnet devices run Spillup."}
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
        return (
          <Row
            key={d.name}
            icon={<OsTile os={d.os} dim={!d.online} />}
            title={
              <span className="flex items-center gap-2">
                {d.name}
                {!d.yours ? <span className="text-xs text-muted">Shared with you</span> : null}
              </span>
            }
            subtitle={`${osName(d.os)} · ${d.ip}`}
            value={
              <span className="flex items-center gap-1.5 text-[13px]">
                <Dot tone={d.online && d.spillup ? "success" : "muted"} />
                {!d.online ? "Offline" : d.spillup ? (d.hold ? holdName[d.hold] : "Spillup") : "Tailnet device"}
              </span>
            }
          />
        );
      })}
    </Group>
  );
}
