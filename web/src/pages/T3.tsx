import { Button, Spinner } from "@heroui/react";
import { Link2, Share2, ShieldCheck, Terminal } from "lucide-react";
import { useOverview } from "../App";
import { Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, osTile } from "../components/ui";
import { api } from "../lib/api";
import { useAction, useQuery } from "../lib/hooks";

/** The machines a T3 Code is linked to, read from `t3 environment list --json`. */
function linkedHosts(linked?: string): string[] {
  if (!linked) return [];
  return [...linked.matchAll(/\/\/([^:/"]+):\d+/g)].map((m) => m[1]);
}

export function T3Page() {
  const { data } = useOverview();
  const t3 = useQuery(api.t3, 15000);
  const machines = useQuery(api.machines, 15000);
  const { pending, run } = useAction();
  const hosts = linkedHosts(t3.data?.linked);
  const others = (machines.data?.machines ?? []).filter((m) => !m.is_self);

  return (
    <>
      <LargeTitle
        title="T3 Code"
        subtitle="Agents on any machine can hand work to any other: spillup keeps every machine's T3 Code linked."
        actions={
          data?.can_manage ? (
            <Button size="sm" isPending={pending === "sync"} onPress={() => run("sync", api.t3Sync, t3.reload)}>
              {({ isPending }) => (
                <>
                  {isPending ? <Spinner size="sm" color="current" /> : <Link2 className="size-3.5" />} Link All Machines Now
                </>
              )}
            </Button>
          ) : null
        }
      />
      {!t3.data ? (
        <GroupSkeleton rows={3} />
      ) : !t3.data.installed ? (
        <Group footer="Install T3 Code from t3.codes, then run setup again: it starts T3 at boot and links it to your other machines.">
          <Row icon={<IconTile icon={Share2} color="gray" />} title="T3 Code isn't installed on this machine" />
          <Row icon={<IconTile icon={Terminal} color="graphite" />} title="Then" accessory={<Mono copy>spinup t3 setup</Mono>} />
        </Group>
      ) : (
        <>
          <Group>
            <Row icon={<IconTile icon={Share2} color="indigo" />} title="T3 Code here" value={<span className="flex items-center gap-1.5"><Dot tone="success" /> Port {t3.data.port}</span>} />
            <Row icon={<IconTile icon={ShieldCheck} color="green" />} title="Access for linked machines" value="Full access" />
          </Group>
          <Group
            title="Linked Machines"
            footer={
              t3.data.problem ??
              "The machine holding the accounts links new machines within five minutes. Change a link's access in T3 → Settings → Sharing & access."
            }
          >
            {others.length ? (
              others.map((m) => {
                const linked = hosts.includes(m.ip ?? "") || hosts.includes(m.name);
                const t = osTile(m.os);
                return (
                  <Row
                    key={m.name}
                    icon={<IconTile icon={t.icon} color={m.online ? t.color : "gray"} />}
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
          <Group title="From the Command Line">
            <Row title="Status" accessory={<Mono copy>spinup t3</Mono>} />
            <Row title="Start T3 at boot, with linking on" accessory={<Mono copy>spinup t3 setup</Mono>} />
            <Row title="Link every machine now" accessory={<Mono copy>spinup t3 link</Mono>} />
          </Group>
        </>
      )}
    </>
  );
}
