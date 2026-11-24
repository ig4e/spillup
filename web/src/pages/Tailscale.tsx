import { Alert } from "@heroui/react";
import { ExternalLink, Globe, KeyRound, Network, RefreshCw, SquareTerminal, Waypoints } from "lucide-react";
import { useEffect, useState } from "react";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, Toggle, osName } from "../components/ui";
import { BrandTile, OsTile } from "../components/brand";
import { TaskLog } from "../components/TaskProgress";
import type { SetupProgress } from "../lib/types";
import { api } from "../lib/api";
import { relative } from "../lib/format";
import { useAction, useQuery, useRemote } from "../lib/hooks";

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
            <Row icon={<BrandTile brand="tailscale" icon={Waypoints} />} title="This machine" value={<span className="flex items-center gap-1.5"><Dot tone={self?.online ? "success" : "danger"} /> {self?.name ?? data?.name ?? "—"}</span>} />
            <Row icon={<IconTile icon={Globe} color="blue" />} title="Address" accessory={self?.ip ? <Mono copy>{self.ip}</Mono> : <span className="text-muted">—</span>} />
            {self?.dns_name ? <Row icon={<IconTile icon={Network} color="teal" />} title="Name on the tailnet" accessory={<Mono copy>{self.dns_name}</Mono>} /> : null}
            <Row icon={<IconTile icon={ExternalLink} color="gray" />} title="Admin Console" tone="accent" onPress={() => window.open("https://login.tailscale.com/admin/machines", "_blank", "noopener")} />
          </Group>
          <SSHGroup />
          {expiring.length ? (
            <Group title="Keys Expiring Soon" footer="A machine whose key expires drops off the tailnet until someone signs in on it. Disable key expiry for your own machines in the admin console.">
              {expiring.map((m) => (
                <Row key={m.name} icon={<IconTile icon={KeyRound} color="orange" />} title={m.name} value={`expires ${relative(m.key_expiry)}`} />
              ))}
            </Group>
          ) : null}
          <Group title="Devices" footer={`${all.filter((m) => m.online).length} of ${all.length} online. spillup reads this list from Tailscale; there's nothing to keep up to date.`}>
            {all.map((m) => {
              return (
                <Row
                  key={m.name}
                  icon={<OsTile os={m.os} dim={!m.online} />}
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

/** SSH between your machines: this one's server, and `ssh <name>` for every other. */
function SSHGroup() {
  const ssh = useQuery(api.ssh, 30000);
  const remote = useRemote();
  const { pending, run } = useAction();
  const [progress, setProgress] = useState<SetupProgress | null>(null);
  useEffect(() => {
    if (!progress?.running) return;
    const id = setInterval(async () => setProgress(await api.setupProgress()), 1000);
    return () => clearInterval(id);
  }, [progress?.running]);
  const task = !!progress;
  const st = ssh.data;
  if (!st) return ssh.loading ? <GroupSkeleton rows={2} /> : null;
  const server = st.server === "tailscale" ? "Tailscale SSH" : "OpenSSH Server";
  const how =
    st.server === "tailscale"
      ? "Tailscale SSH signs you in with your Tailscale account: no keys or passwords."
      : "Windows' own SSH server, let in through the firewall from your tailnet only; your machines' keys sign you in.";
  return (
    <>
      <Group
        title="SSH"
        footer={`${how} spillup names every machine in ~/.ssh/config, so \`ssh <name>\` works from any of them, at home or away.${st.problem ? ` ${st.problem}` : ""}`}
      >
        <Row
          icon={<IconTile icon={SquareTerminal} color="graphite" />}
          title={server}
          subtitle={st.on && st.address ? `ssh ${st.user}@${st.address}` : st.on ? "On" : "Off"}
          accessory={
            remote ? (
              <span className="text-[13px] text-muted">{st.on ? "On" : "Off"}</span>
            ) : (
              <Toggle
                label={server}
                isSelected={st.on}
                isDisabled={task}
                onChange={async (on) => {
                  if (await run("ssh", () => api.sshEnable(on))) setProgress({ running: true, done: false, log: [] });
                }}
              />
            )
          }
        />
        {st.hosts.map((h) => (
          <Row key={h} icon={<IconTile icon={Network} color="teal" />} title={h} accessory={<Mono copy>{`ssh ${h}`}</Mono>} />
        ))}
        <Row
          icon={<IconTile icon={RefreshCw} color="gray" />}
          title={pending === "sync" ? "Asking your machines…" : "Refresh SSH Names and Keys"}
          tone="accent"
          busy={pending === "sync"}
          onPress={pending ? undefined : () => run("sync", api.sshSync, ssh.reload)}
        />
      </Group>
      {progress ? (
        <TaskLog
          progress={progress}
          hint={st.server === "tailscale" ? "Enter your password when asked: Tailscale needs it to change its settings." : "Approve the administrator prompt on this machine."}
          onRetry={() => setProgress(null)}
          onDone={() => {
            setProgress(null);
            void ssh.reload();
          }}
        />
      ) : null}
    </>
  );
}
