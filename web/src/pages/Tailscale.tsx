import { Alert } from "@heroui/react";
import { ExternalLink, Globe, KeyRound, Network, Waypoints } from "lucide-react";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, osName, osTile } from "../components/ui";
import { api } from "../lib/api";
import { relative } from "../lib/format";
import { useQuery } from "../lib/hooks";

export function TailscalePage() {
  const { data } = useOverview();
  const machines = useQuery(api.machines, 10000);
  const all = machines.data?.machines ?? [];
  const self = all.find((m) => m.is_self);
  const expiring = all.filter((m) => m.key_expiry && new Date(m.key_expiry).getTime() - Date.now() < 14 * 86400_000);

  return (
    <>
      <LargeTitle title="Tailscale" subtitle="The private network your machines use to reach each other, at home or away." />
      {machines.data?.problem ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>Tailscale didn’t answer</Alert.Title>
            <Alert.Description>{machines.data.problem}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
      {!machines.data ? (
        <GroupSkeleton rows={3} />
      ) : (
        <>
          <Group>
            <Row icon={<IconTile icon={Waypoints} color="graphite" />} title="This machine" value={<span className="flex items-center gap-1.5"><Dot tone={self?.online ? "success" : "danger"} /> {self?.name ?? data?.name ?? "—"}</span>} />
            <Row icon={<IconTile icon={Globe} color="blue" />} title="Address" accessory={self?.ip ? <Mono copy>{self.ip}</Mono> : <span className="text-muted">—</span>} />
            {self?.dns_name ? <Row icon={<IconTile icon={Network} color="teal" />} title="Name on the tailnet" accessory={<Mono copy>{self.dns_name}</Mono>} /> : null}
            <Row icon={<IconTile icon={ExternalLink} color="gray" />} title="Admin Console" tone="accent" onPress={() => window.open("https://login.tailscale.com/admin/machines", "_blank", "noopener")} />
          </Group>
          {expiring.length ? (
            <Group title="Keys Expiring Soon" footer="A machine whose key expires drops off the tailnet until someone signs in on it. Disable key expiry for your own machines in the admin console.">
              {expiring.map((m) => (
                <Row key={m.name} icon={<IconTile icon={KeyRound} color="orange" />} title={m.name} value={`expires ${relative(m.key_expiry)}`} />
              ))}
            </Group>
          ) : null}
          <Group title="Devices" footer={`${all.filter((m) => m.online).length} of ${all.length} online. spillup reads this list from Tailscale; there's nothing to keep up to date.`}>
            {all.map((m) => {
              const t = osTile(m.os);
              return (
                <Row
                  key={m.name}
                  icon={<IconTile icon={t.icon} color={m.online ? t.color : "gray"} />}
                  title={m.name}
                  subtitle={`${osName(m.os)} · ${m.ip ?? "—"}`}
                  value={<span className="flex items-center gap-1.5"><Dot tone={m.expired ? "danger" : m.online ? "success" : "muted"} /> {m.expired ? "Key expired" : m.online ? "Online" : `Seen ${relative(m.last_seen)}`}</span>}
                />
              );
            })}
          </Group>
        </>
      )}
    </>
  );
}
