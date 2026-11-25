import { Smartphone, Tablet } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../lib/api";
import { useQuery, useRemote } from "../lib/hooks";
import { hasAgentLink, mobilePeer } from "../lib/t3";
import { QR } from "./QR";
import { Dot, Group, IconTile, Mono, Row } from "./ui";

export function PhoneConnections({ address, host, agentHosts, installed }: { address?: string; host?: string; agentHosts: Set<string>; installed: boolean }) {
  const current = useRemote();
  const machines = useQuery(api.localMachines, 15_000);
  const peers = (machines.data?.machines ?? []).filter(m =>
    (current ? m.name !== current : !m.is_self) && m.online && !m.expired && m.yours &&
    !mobilePeer(m) && (m.is_self || m.hold || hasAgentLink(m, agentHosts)));
  if (!installed && peers.length === 0) return null;
  return (
    <Group title="Phones & tablets">
      {installed ? <HostPhones target={current} host={host || current || "this machine"} /> : null}
      {peers.map(peer => <HostPhones key={peer.name} target={peer.is_self ? null : peer.name} host={peer.name} />)}
      {address ? <PairPhone address={address} host={host || current || "this machine"} /> : null}
    </Group>
  );
}

function HostPhones({ target, host }: { target: string | null; host: string }) {
  const q = useQuery(api.t3PhonesOf(target), 30_000);
  if (!q.data) return <Row title={host} value={q.error ? "Pairing status unavailable" : "Checking phone pairings…"} />;
  if (!q.data.phones.length) return <Row title={host} value={q.refreshError ? "Pairing status unavailable" : "No paired phones or tablets"} />;
  return q.data.phones.map(phone => {
    const title = phone.name || (phone.kind === "tablet" ? "Tablet" : "Phone");
    return <Row key={JSON.stringify([phone.name, phone.kind])} title={title}
      icon={<IconTile icon={phone.kind === "tablet" ? Tablet : Smartphone} color="blue" />}
      subtitle={phone.count > 1 ? `${host} · ${phone.count} sessions` : host}
      value={<span className="inline-flex items-center gap-1.5"><Dot tone={q.refreshError ? "muted" : "success"} />{q.refreshError ? "Paired at last check" : "Paired"}</span>} />;
  });
}

function PairPhone({ address, host }: { address: string; host: string }) {
  const [link, setLink] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  useEffect(() => {
    if (!link) return;
    const timer = window.setTimeout(() => setLink(null), 10 * 60_000);
    return () => window.clearTimeout(timer);
  }, [link]);
  const pair = async () => {
    setBusy(true);
    setProblem("");
    try {
      const { code } = await api.t3Pair("phone");
      setLink(`${address}/pair#token=${encodeURIComponent(code)}`);
    } catch (err) {
      setProblem((err as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return <>
    <Row icon={<IconTile icon={Smartphone} color="blue" />} title={busy ? "Making a link…" : `Pair a phone with ${host}`}
      tone="accent" busy={busy} onPress={busy ? undefined : () => void pair()} />
    {problem ? <p className="px-4 py-2 text-sm text-danger" role="alert">{problem}</p> : null}
    {link ? <div className="flex flex-col items-center gap-4 px-4 py-4 sm:flex-row">
      <QR value={link} size={148} />
      <div className="min-w-0 flex-1 text-center sm:text-left">
        <div className="text-[15px] font-semibold">Scan with your phone</div>
        <div className="mt-1 text-[13px] text-muted">Connect to Tailscale, then scan this one-time link within ten minutes.</div>
        <div className="mt-2.5 flex justify-center sm:justify-start"><Mono copy compact>{link}</Mono></div>
        <button type="button" className="mt-2 min-h-11 text-sm text-accent" onClick={() => setLink(null)}>Hide link</button>
      </div>
    </div> : null}
  </>;
}
