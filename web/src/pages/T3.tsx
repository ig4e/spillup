import { Button } from "../components/Button";
import { Download, Link2, RefreshCw, Share2, ShieldCheck, Smartphone } from "lucide-react";
import { useState } from "react";
import { QR } from "../components/QR";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row } from "../components/ui";
import { BrandTile, OsTile } from "../components/brand";
import { api, currentRemote } from "../lib/api";
import { useAction, useQuery } from "../lib/hooks";

/** The machines a T3 Code is linked to, read from `t3 environment list --json`. */
function linkedHosts(linked?: string): string[] {
  if (!linked) return [];
  return [...linked.matchAll(/\/\/([^:/"]+):\d+/g)].map((m) => m[1].toLowerCase());
}

export function T3Page() {
  const { data } = useOverview();
  const t3 = useQuery(api.t3, 15000);
  const machines = useQuery(api.machines, 15000);
  const { pending, run } = useAction();
  const hosts = linkedHosts(t3.data?.linked);
  const others = (machines.data?.machines ?? []).filter((m) => !m.is_self);

  const localDesktop = !currentRemote() && ["localhost", "127.0.0.1", "[::1]"].includes(window.location.hostname);
  const desktopPeers = others.filter(m => m.online && !m.expired && m.yours && (m.hold || hosts.includes(m.ip ?? "") || hosts.includes(m.dns_name?.replace(/\.$/, "").toLowerCase() ?? "")));
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
      {!t3.data ? (
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
              t3.data.problem ??
              "These links let agents delegate work between machines. Spillup checks them every five minutes."
            }
          >
            {others.length ? (
              others.map((m) => {
                const dns = m.dns_name?.replace(/\.$/, "").toLowerCase();
                const linked = hosts.some((h) => h === m.ip || h === m.name.toLowerCase() || h === dns || h.startsWith(`${m.name.toLowerCase()}.`));
                return (
                  <Row
                    key={m.name}
                    icon={<OsTile os={m.os} dim={!m.online} />}
                    title={m.name}
                    subtitle={m.online ? m.ip : "Offline"}
                    value={<span className="flex items-center gap-1.5"><Dot tone={linked ? "success" : "muted"} /> {linked ? "Linked" : "Not linked yet"}</span>}
                  />
                );
              })
            ) : (
              <Row title={<span className="text-muted">No other machines on your tailnet yet</span>} />
            )}
          </Group>
          {t3.data.address ? <PhoneGroup address={t3.data.address} /> : null}
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

/** Pairs a phone or tablet with this T3 Code over Tailscale: a one-time link, as a QR code for its camera. */
function PhoneGroup({ address }: { address: string }) {
  const [link, setLink] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const pair = async () => {
    setBusy(true);
    setProblem(null);
    try {
      const { code } = await api.t3Pair("phone");
      setLink(`${address}/pair#token=${encodeURIComponent(code)}`);
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Group
      title="Your Phone"
      footer={
        problem ??
        "Install Tailscale on your phone and sign in with the same account; then T3 Code opens on it at home or away, with every machine linked here. The link works once, for ten minutes."
      }
    >
      {link ? (
        <div className="flex flex-col items-center gap-4 px-4 py-4 sm:flex-row">
          <QR value={link} size={148} />
          <div className="min-w-0 flex-1 text-center sm:text-left">
            <div className="text-[15px] font-semibold">Scan with Your Phone</div>
            <div className="mt-1 text-[13px] text-muted">It opens T3 Code signed in. Then Share → Add to Home Screen.</div>
            <div className="mt-2.5 flex justify-center sm:justify-start">
              <Mono copy compact>{link}</Mono>
            </div>
          </div>
        </div>
      ) : null}
      <Row
        icon={<IconTile icon={Smartphone} color="blue" />}
        title={busy ? "Making a link…" : link ? "Make Another Link" : "Pair Your Phone"}
        tone="accent"
        busy={busy}
        onPress={busy ? undefined : () => void pair()}
      />
    </Group>
  );
}
