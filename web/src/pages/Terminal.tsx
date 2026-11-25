import "@xterm/xterm/css/xterm.css";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm, type ITheme } from "@xterm/xterm";
import { Plus, SquareTerminal } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useOverview } from "../App";
import { Button } from "../components/Button";
import { LargeTitle, Segmented } from "../components/ui";
import { api, setRemote } from "../lib/api";
import { useQuery, useRemote } from "../lib/hooks";

const light: ITheme = {
  background: "#ffffff",
  foreground: "#1d1d1f",
  cursor: "#1d1d1f",
  selectionBackground: "#b4d5fe",
};
const dark: ITheme = {
  background: "#1e1e1e",
  foreground: "#e5e5e7",
  cursor: "#e5e5e7",
  selectionBackground: "#3f638b",
};

const isDark = () => document.documentElement.classList.contains("dark") || (!document.documentElement.classList.contains("light") && matchMedia("(prefers-color-scheme: dark)").matches);

type Status = { kind: "connecting" } | { kind: "open" } | { kind: "ended"; why: string };

export function TerminalPage() {
  const { data, loading } = useOverview();
  const remote = useRemote();
  const machines = useQuery(api.localManagedMachines, 15000);
  const local = useQuery(api.localOverview);
  const [session, setSession] = useState(0);
  const managed = machines.data?.machines ?? [];
  const selected = managed.find(m => remote ? m.name === remote : m.is_self);
  const choices = managed.filter(m => m.online && !m.expired && (m.is_self ? local.data && local.data.settings.terminals !== false : m.terminals === true))
    .map(m => ({ id: m.is_self ? "__self" : m.name, label: m.name }));
  const self = local.data?.name || "This Machine";
  const off = data?.settings.terminals === false || selected?.terminals === false;
  const allowed = !!data && !off && !!selected && selected.online && !selected.expired && (!remote || selected.terminals === true);
  const checking = loading || machines.loading;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <LargeTitle
        bleed
        title="Terminal"
        center={
          choices.length > 1 || (choices.length > 0 && !allowed) ? (
            <Segmented label="Machine" value={remote ?? "__self"} onChange={(id) => setRemote(id === "__self" ? null : id)} options={choices} />
          ) : undefined
        }
        actions={
          <Button size="sm" variant="secondary" isDisabled={!allowed || !!machines.refreshError} onPress={() => setSession((n) => n + 1)}>
            <Plus className="size-4" /> New Session
          </Button>
        }
      />
      {checking || !data || (!off && !allowed) ? <div className="flex flex-1 items-center justify-center text-sm text-muted">{checking ? "Checking terminal access…" : "No terminal available on this machine."}</div> : off ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-center text-muted">
          <SquareTerminal className="size-10" />
          <div className="text-[15px] font-semibold text-foreground">Terminals are off on {data?.name}</div>
          <div className="max-w-sm text-[13px]">Turn them on in {data?.name}’s Settings → Your Devices.</div>
        </div>
      ) : (
        <TerminalView key={`${remote ?? ""}#${session}`} target={remote} machine={data?.name ?? self} onNew={() => setSession((n) => n + 1)} />
      )}
    </div>
  );
}

function TerminalView({ target, machine, onNew }: { target: string | null; machine: string; onNew: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const term = useRef<XTerm | null>(null);
  const [status, setStatus] = useState<Status>({ kind: "connecting" });

  useEffect(() => {
    const session = api.terminalOn(target);
    const el = box.current;
    if (!el) return;
    const xt = new XTerm({
      fontFamily: '"SF Mono", ui-monospace, Menlo, "JetBrains Mono", monospace',
      fontSize: 13,
      lineHeight: 1.15,
      cursorBlink: true,
      allowProposedApi: true,
      scrollback: 5000,
      theme: isDark() ? dark : light,
    });
    term.current = xt;
    const fit = new FitAddon();
    xt.loadAddon(fit);
    xt.open(el);
    fit.fit();

    const stop = new AbortController();
    let id = "";
    let sending = Promise.resolve();
    const send = (data: string) => {
      if (!id) return;
      const target = id;
      sending = sending.then(() => session.input(target, data).catch(() => undefined));
    };
    const typing = xt.onData(send);
    const resizing = xt.onResize(({ cols, rows }) => id && void session.resize(id, cols, rows).catch(() => undefined));
    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(el);
    const scheme = matchMedia("(prefers-color-scheme: dark)");
    const retheme = () => (xt.options.theme = isDark() ? dark : light);
    scheme.addEventListener("change", retheme);

    const attach = async (tries: number): Promise<void> => {
      try {
        const stream = await session.output(id, stop.signal);
        setStatus({ kind: "open" });
        xt.focus();
        const reader = stream.getReader();
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          if (value?.length) xt.write(value);
        }
        if (!stop.signal.aborted && tries > 0) {
          setStatus({ kind: "connecting" });
          await new Promise((r) => setTimeout(r, 800));
          return attach(tries - 1);
        }
        if (!stop.signal.aborted) setStatus({ kind: "ended", why: "The shell exited." });
      } catch (e) {
        if (stop.signal.aborted) return;
        const status = (e as { status?: number }).status;
        if (status !== 404 && status !== 403 && tries > 0) {
          setStatus({ kind: "connecting" });
          await new Promise((r) => setTimeout(r, 1500));
          return attach(tries - 1);
        }
        setStatus({ kind: "ended", why: status === 404 ? "The shell exited." : (e as Error).message });
      }
    };

    void (async () => {
      try {
        ({ id } = await session.start(xt.cols, xt.rows));
        if (stop.signal.aborted) {
          void session.close(id).catch(() => undefined);
          return;
        }
        await attach(5);
      } catch (e) {
        if (!stop.signal.aborted) setStatus({ kind: "ended", why: (e as Error).message });
      }
    })();

    return () => {
      stop.abort();
      if (id) void session.close(id).catch(() => undefined);
      typing.dispose();
      resizing.dispose();
      observer.disconnect();
      scheme.removeEventListener("change", retheme);
      xt.dispose();
      term.current = null;
    };
  }, [target]);

  const press = (data: string) => {
    term.current?.focus();
    term.current?.input(data, true);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-[#ffffff] shadow-[0_0_0_0.5px_var(--border),0_1px_3px_rgb(0_0_0/0.06)] dark:bg-[#1e1e1e]">
      <div className="relative min-h-0 flex-1 p-2.5">
        <div ref={box} className="h-full w-full" />
        {status.kind !== "open" ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background/70 text-center backdrop-blur-sm">
            <div className="text-[14px] font-medium">{status.kind === "connecting" ? `Connecting to ${machine}…` : status.why}</div>
            {status.kind === "ended" ? (
              <Button size="sm" onPress={onNew}>
                New Session
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      <KeyRow onKey={press} />
    </div>
  );
}

const keys: { label: string; data: string }[] = [
  { label: "esc", data: "\x1b" },
  { label: "tab", data: "\t" },
  { label: "ctrl-c", data: "\x03" },
  { label: "ctrl-d", data: "\x04" },
  { label: "←", data: "\x1b[D" },
  { label: "↑", data: "\x1b[A" },
  { label: "↓", data: "\x1b[B" },
  { label: "→", data: "\x1b[C" },
];

/** Keys a phone's keyboard lacks, shown only on touch screens. */
function KeyRow({ onKey }: { onKey: (data: string) => void }) {
  return (
    <div className="flex gap-1.5 overflow-x-auto border-t border-separator px-2.5 py-2 [@media(pointer:fine)]:hidden">
      {keys.map((k) => (
        <button
          key={k.label}
          type="button"
          onPointerDown={(e) => e.preventDefault()}
          onClick={() => onKey(k.data)}
          className="min-h-9 shrink-0 rounded-lg bg-foreground/[0.07] px-3 font-mono text-[13px] active:bg-foreground/[0.15]"
        >
          {k.label}
        </button>
      ))}
    </div>
  );
}
