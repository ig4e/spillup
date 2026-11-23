import { Button, Switch, toast } from "@heroui/react";
import { Bot, Code2, LogIn, Orbit, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { api } from "../lib/api";
import { describeLogin, relative } from "../lib/format";
import { useQuery, useRemote } from "../lib/hooks";
import type { Family, OwnState } from "../lib/types";
import { Group, IconTile, Row, type Tint } from "./ui";

const families: Record<Family, { name: string; provider: string; types: string[]; icon: LucideIcon; color: Tint }> = {
  claude: { name: "Claude", provider: "Claude", types: ["claude"], icon: Bot, color: "orange" },
  gpt: { name: "GPT & Codex", provider: "ChatGPT", types: ["codex"], icon: Code2, color: "graphite" },
  gemini: { name: "Gemini", provider: "Google (Antigravity)", types: ["antigravity", "gemini"], icon: Orbit, color: "indigo" },
};

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Each machine may sign in for itself per model family, so no two machines ever refresh one token. */
export function OwnLogins() {
  const own = useQuery(api.own, 5000);
  const [busy, setBusy] = useState<string | null>(null);
  const remote = useRemote();
  const data = own.data;
  if (!data) return null;

  const toggle = async (f: Family, on: boolean) => {
    setBusy(f);
    try {
      const next = on ? [...data.families, f] : data.families.filter((x) => x !== f);
      toast.success((await api.setOwn(next)).message);
      await own.reload();
    } catch (e) {
      toast.danger((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const signIn = async (f: Family) => {
    const tab = window.open("about:blank", "_blank");
    setBusy(`signin-${f}`);
    try {
      const link = await api.signIn(f);
      if (tab) tab.location.href = link.url;
      else window.open(link.url, "_blank");
      const id = toast(`Finish signing in to ${families[f].provider} in the new tab…`, { isLoading: true, timeout: 0 });
      for (let i = 0; i < 150; i++) {
        await wait(2000);
        const st = await api.signInStatus(link.state);
        if (st.status === "ok") {
          toast.close(id);
          toast.success(`Signed in to ${families[f].provider} on this machine.`);
          break;
        }
        if (st.status === "error") {
          toast.close(id);
          toast.danger(st.error ?? "The sign-in failed.");
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

  return (
    <Group
      title="Sign In Separately on This Machine"
      footer={
        <>
          Turn a model family on to stop sharing it: this machine signs in with its own account and never uses, or refreshes, the
          shared login. Use it when a provider logs out the other sessions on refresh, as Claude does. Other models keep using the
          shared accounts.
          {data.families.length && !data.running ? " This machine's own CLIProxyAPI isn't running yet." : ""}
        </>
      }
    >
      {data.available.map((f) => {
        const look = families[f];
        const on = data.families.includes(f);
        const mine = loginsFor(data, f);
        return (
          <div key={f} className="divide-y divide-separator">
            <Row
              icon={<IconTile icon={look.icon} color={look.color} />}
              title={`${look.name} models`}
              subtitle={on ? (mine.length ? `Own sign-in · ${mine.map((l) => describeLogin(l.name, l.type).label).join(", ")}` : "Own sign-in · not signed in yet") : "Shared accounts"}
              accessory={
                <Switch size="sm" aria-label={`${look.name} uses this machine's own sign-in`} isSelected={on} isDisabled={busy === f} onChange={(v) => toggle(f, v)}>
                  <Switch.Control>
                    <Switch.Thumb />
                  </Switch.Control>
                </Switch>
              }
            />
            {on
              ? mine.map((l) => (
                  <Row
                    key={l.name}
                    icon={<span className="size-7" />}
                    title={describeLogin(l.name, l.type).label}
                    subtitle={`Signed in on this machine · refreshed ${relative(l.refreshed)}`}
                    accessory={
                      <Button size="sm" variant="ghost" className="text-danger" isPending={busy === l.name} onPress={() => signOut(l.name)}>
                        Sign Out
                      </Button>
                    }
                  />
                ))
              : null}
            {on && remote ? (
              <Row icon={<span className="size-7" />} title={<span className="text-muted">Sign in from {remote} itself: the sign-in finishes in a browser there.</span>} />
            ) : null}
            {on && !remote ? (
              <Row
                icon={<span className="flex size-7 items-center justify-center text-accent"><LogIn className="size-4" /></span>}
                title={busy === `signin-${f}` ? "Waiting for the browser…" : `Sign In to ${look.provider}${mine.length ? " with Another Account" : ""}…`}
                tone="accent"
                onPress={busy ? undefined : () => signIn(f)}
              />
            ) : null}
          </div>
        );
      })}
    </Group>
  );
}

function loginsFor(data: OwnState, f: Family) {
  return data.logins.filter((l) => families[f].types.includes((l.type ?? describeLogin(l.name).provider).toLowerCase()));
}
