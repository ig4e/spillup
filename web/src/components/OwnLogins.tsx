import { Button, toast } from "@heroui/react";
import { LogIn } from "lucide-react";
import { useState } from "react";
import { api } from "../lib/api";
import { describeLogin, relative } from "../lib/format";
import { useQuery, useRemote } from "../lib/hooks";
import { providerLook } from "../lib/providers";
import type { Account, OwnState, ProviderInfo } from "../lib/types";
import { Group, GroupSkeleton, IconTile, Row, Toggle } from "./ui";

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Each machine may sign in for itself per provider, so no two machines ever refresh one token. */
export function OwnLogins() {
  const own = useQuery(api.own, 5000);
  const [busy, setBusy] = useState<string | null>(null);
  const remote = useRemote();
  const data = own.data;
  if (!data) return <GroupSkeleton rows={4} />;

  const toggle = async (p: string, on: boolean) => {
    setBusy(p);
    try {
      const next = on ? [...data.providers, p] : data.providers.filter((x) => x !== p);
      toast.success((await api.setOwn(next)).message);
      await own.reload();
    } catch (e) {
      toast.danger((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const signIn = async (p: ProviderInfo) => {
    const tab = window.open("about:blank", "_blank");
    setBusy(`signin-${p.id}`);
    try {
      const link = await api.signIn(p.id);
      if (tab) tab.location.href = link.url;
      else window.open(link.url, "_blank");
      const id = toast(`Finish signing in to ${p.name} in the new tab…`, { isLoading: true, timeout: 0 });
      for (let i = 0; i < 150; i++) {
        await wait(2000);
        const st = await api.signInStatus(link.state);
        if (st.status === "ok" || st.status === "error") {
          toast.close(id);
          if (st.status === "ok") toast.success(`Signed in to ${p.name} on this machine.`);
          else toast.danger(st.error ?? "The sign-in failed.");
          break;
        }
      }
      await own.reload();
    } catch (e) {
      tab?.close();
      toast.danger((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const signOut = async (name: string) => {
    setBusy(name);
    try {
      toast.success((await api.signOut(name)).message);
      await own.reload();
    } catch (e) {
      toast.danger((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const enabled = data.available.filter((p) => data.providers.includes(p.id));
  const others = data.available.filter((p) => !data.providers.includes(p.id));

  return (
    <>
      {enabled.length ? (
        <Group
          title="Signed In Separately"
          footer={data.running ? "These go to this machine's own CLIProxyAPI. Everything else uses the shared accounts." : "This machine's own CLIProxyAPI is starting…"}
        >
          {enabled.flatMap((p) => providerRows(p, data, true))}
        </Group>
      ) : null}
      <Group
        title={enabled.length ? "Other Providers" : "Providers"}
        footer="Switch a provider on to stop sharing it: this machine signs in with its own account and never uses, or refreshes, the shared login. Use it when a provider logs other sessions out on refresh."
      >
        {others.flatMap((p) => providerRows(p, data, false))}
      </Group>
    </>
  );

  function providerRows(p: ProviderInfo, state: OwnState, on: boolean) {
    const look = providerLook(p.id);
    const mine = loginsFor(state.logins, p.id);
    const rows = [
      <Row
        key={p.id}
        icon={<IconTile icon={look.icon} color={look.color} />}
        title={p.name}
        subtitle={on ? (mine.length ? `${mine.length} account${mine.length === 1 ? "" : "s"} on this machine` : "Not signed in yet") : "Shared accounts"}
        accessory={<Toggle label={`${p.name} uses this machine's own sign-in`} isSelected={on} isDisabled={busy === p.id} onChange={(v) => toggle(p.id, v)} />}
      />,
    ];
    if (!on) return rows;
    for (const l of mine) {
      rows.push(
        <Row
          key={l.name}
          icon={<span className="size-7" />}
          title={describeLogin(l.name, l.type).label}
          subtitle={`Refreshed ${relative(l.refreshed)}`}
          accessory={
            remote ? undefined : (
              <Button size="sm" variant="ghost" className="text-danger" isPending={busy === l.name} onPress={() => signOut(l.name)}>
                Sign Out
              </Button>
            )
          }
        />,
      );
    }
    if (remote) {
      rows.push(<Row key={`${p.id}-remote`} icon={<span className="size-7" />} title={<span className="text-muted">Sign in on {remote} itself: the sign-in finishes in a browser there.</span>} />);
    } else if (p.sign_in) {
      rows.push(
        <Row
          key={`${p.id}-signin`}
          icon={
            <span className="flex size-7 items-center justify-center text-accent">
              <LogIn className="size-4" />
            </span>
          }
          title={busy === `signin-${p.id}` ? "Waiting for the browser…" : `Sign In to ${p.name}${mine.length ? " with Another Account" : ""}…`}
          tone="accent"
          onPress={busy ? undefined : () => signIn(p)}
        />,
      );
    } else {
      rows.push(<Row key={`${p.id}-panel`} icon={<span className="size-7" />} title="Add its login in CLIProxyAPI’s dashboard" tone="accent" to="/proxy" />);
    }
    return rows;
  }
}

function loginsFor(logins: Account[], provider: string) {
  return logins.filter((l) => (l.type ?? describeLogin(l.name).provider).toLowerCase() === provider);
}
