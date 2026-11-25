import { Button } from "../components/Button";
import { Download, Link2, RefreshCw, Share2, ShieldCheck } from "lucide-react";
import { PhoneConnections } from "../components/PhoneConnections";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Row } from "../components/ui";
import { BrandTile, OsTile } from "../components/brand";
import { api, currentRemote } from "../lib/api";
import { useAction, useQuery } from "../lib/hooks";
import { agentHosts, hasAgentLink, mobilePeer } from "../lib/t3";

export function T3Page() {
  const { data } = useOverview();
  const t3 = useQuery(api.t3, 15000);
  const machines = useQuery(api.machines, 15000);
  const { pending, run } = useAction();
  const hosts = agentHosts(t3.data?.linked);
  const others = (machines.data?.machines ?? []).filter(m => !m.is_self && (!mobilePeer(m) || hasAgentLink(m, hosts)));

  const localDesktop = !currentRemote() && ["localhost", "127.0.0.1", "[::1]"].includes(window.location.hostname);
  const desktopPeers = others.filter(m => m.online && !m.expired && m.yours && (m.hold || hasAgentLink(m, hosts)));
  const connectDesktop = async (name: string) => {
    const { link } = await api.t3Desktop(name);
    if (!link.startsWith("t3code://pair?")) throw new Error("The machine returned an invalid T3 app link.");
    window.location.assign(link);
    return { message: `Confirm ${name} in T3 Code to add it to your machine picker.` };
  };
  return (
    <>
      <LargeTitle
        title="T3 Code"
        actions={
          data?.can_manage ? (
            <Button size="sm" isPending={pending === "sync"} onPress={() => run("sync", api.t3Sync, t3.reload)}>
              {pending === "sync" ? null : <Link2 className="size-3.5" />} Link All Machines Now
            </Button>
          ) : null
        }
      />
      {localDesktop && desktopPeers.length > 0 ? (
        <Group title="Desktop connections" footer="Add a machine to this desktop’s project picker. Confirm its address in T3 Code, then choose Add Project or select the machine before sending a projectless thread.">
          {desktopPeers.map(machine => (
            <Row key={machine.name} icon={<OsTile os={machine.os} />} title={machine.name} value="Connect in T3 Code"
              busy={pending === `desktop-${machine.name}`} onPress={pending ? undefined : () => void run(`desktop-${machine.name}`, () => connectDesktop(machine.name))} />
          ))}
        </Group>
      ) : null}
      {t3.data ? <PhoneConnections address={t3.data.address} installed={t3.data.installed} host={data?.name} agentHosts={hosts} /> : null}
      {!t3.data && t3.error ? (
        <Group><Row title="T3 Code status unavailable" subtitle={t3.error.message} /></Group>
      ) : !t3.data ? (
        <GroupSkeleton rows={3} />
      ) : !t3.data.installed ? (
        <Group
          footer={
            t3.data.app
              ? "T3 Code's app is here, but not the background service spillup needs to keep it running and linked. Setting it up installs T3's command line with T3's own installer, starts it at boot, and turns linking on."
              : "Installs T3 Code's command line with T3's own installer, starts it at boot, and links it to your other machines."
          }
        >
          <Row icon={<BrandTile brand="t3" icon={Share2} dim={!t3.data.app} />} title={t3.data.app ? "T3 Code's app is installed" : "T3 Code isn't installed on this machine"} />
          <Row
            icon={<IconTile icon={Download} color="blue" />}
            title={pending === "setup" ? "Setting up T3 Code…" : t3.data.app ? "Set Up T3 Code" : "Install and Set Up T3 Code"}
            tone="accent"
            busy={pending === "setup"}
            onPress={pending ? undefined : () => run("setup", api.t3Setup, t3.reload)}
          />
        </Group>
      ) : (
        <>
          {t3.data.network_access === false ? (
            <Group footer="The T3 Code app answers only this machine, so your other machines can't link to it. Setting T3 Code up turns on its Network access; then quit and reopen T3 Code once.">
              <Row icon={<IconTile icon={Share2} color="orange" />} title="Other machines can't reach this T3 Code" />
              <Row
                icon={<IconTile icon={Download} color="blue" />}
                title={pending === "setup" ? "Setting up T3 Code…" : "Turn On Network Access"}
                tone="accent"
                busy={pending === "setup"}
                onPress={pending ? undefined : () => run("setup", api.t3Setup, t3.reload)}
              />
            </Group>
          ) : null}
          {t3.data.awaits_restart ? (
            <Group footer="Network access is on, but T3 Code reads it when it starts. Quit and reopen T3 Code here, then link your machines again.">
              <Row icon={<IconTile icon={Share2} color="orange" />} title="Restart T3 Code so other machines can reach it" />
            </Group>
          ) : null}
          <Group>
            <Row icon={<BrandTile brand="t3" icon={Share2} />} title="T3 Code here" value={<span className="flex items-center gap-1.5"><Dot tone="success" /> Port {t3.data.port}</span>} />
            <Row icon={<IconTile icon={ShieldCheck} color="green" />} title="Access for linked machines" value="Full access" />
          </Group>
          <Group
            title="Agent Links"
            footer={
              (t3.refreshError ? "Showing the last link check." : t3.data.problem) ??
              "These links let agents delegate work between machines. Spillup checks them every five minutes."
            }
          >
            {others.length ? (
              others.map((m) => {
                const linked = hasAgentLink(m, hosts);
                return (
                  <Row
                    key={m.name}
                    icon={<OsTile os={m.os} dim={!m.online} />}
                    title={m.name}
                    subtitle={m.online ? m.ip : "Offline"}
                    value={<span className="flex items-center gap-1.5"><Dot tone={linked ? "success" : "muted"} /> {linked ? "Linked" : t3.data?.problem ? "Link status unavailable" : "Not linked yet"}</span>}
                  />
                );
              })
            ) : (
              <Row title={<span className="text-muted">{machines.loading ? "Reading machines…" : machines.error ? "Machine list unavailable" : "No other agent machines"}</span>} />
            )}
          </Group>
          <Group footer="Starts T3's background service at boot again, on your LAN and tailnet, with linking on.">
            <Row
              icon={<IconTile icon={RefreshCw} color="gray" />}
              title={pending === "setup" ? "Setting up T3 Code…" : "Set Up T3 Code Again"}
              tone="accent"
              busy={pending === "setup"}
              onPress={pending ? undefined : () => run("setup", api.t3Setup, t3.reload)}
            />
          </Group>
        </>
      )}
    </>
  );
}
