import { useState } from "react";
import { Lock } from "lucide-react";
import { api } from "../lib/api";
import type { SetupProgress } from "../lib/types";
import { Button } from "./Button";
import { Group, IconTile, Row } from "./ui";

/** A spinup task the dashboard runs, such as setup: its live output, sudo's password questions, and what to do when it ends. */
export function TaskLog({ progress, connectionError, hint, onRetry, onDone }: { progress: SetupProgress; connectionError: string; hint: string; onRetry: () => void; onDone: () => void }) {
  const total = progress.total ?? 0;
  const completed = Math.max(0, (progress.step ?? 0) - 1);
  const elapsed = progress.elapsed_seconds ?? 0;
  const duration = `${Math.floor(elapsed / 60)}m ${elapsed % 60}s`;
  const status = connectionError
    ? `Reconnecting. ${connectionError}`
    : progress.prompt
      ? "Waiting for your password"
      : (progress.quiet_seconds ?? 0) >= 45
        ? "Waiting for more output. Check for a system prompt or sign-in."
        : "";
  return (
    <Group
      title={progress.running ? "Running" : progress.failed ? "Stopped" : "Complete"}
      footer={progress.failed || (progress.running ? hint : undefined)}
    >
      {progress.running ? (
        <div className="flex flex-col gap-2 px-4 py-3">
          <div className="flex items-center justify-between gap-3 text-sm">
            <span>{total ? `Step ${progress.step} of ${total} · ${progress.phase}` : progress.phase || "Waiting for output"}</span>
            <span className="shrink-0 text-muted">{duration}</span>
          </div>
          <progress aria-label="Task progress" max={total || 1} value={total ? completed : undefined} className="h-2 w-full accent-accent" />
          {status ? <p role="status" className="text-xs text-muted">{status}</p> : null}
        </div>
      ) : null}
      {progress.prompt ? <PasswordPrompt key={progress.prompt} prompt={progress.prompt} /> : null}
      <div className="max-h-96 overflow-y-auto px-4 py-3 font-mono text-[12px] leading-relaxed whitespace-pre-wrap">
        {progress.log.length ? progress.log.map((line, i) => <div key={i}>{linkify(line)}</div>) : <span className="text-muted">Starting…</span>}
      </div>
      {!progress.running ? <Row title={progress.failed ? "Try Again" : "Done"} tone="accent" onPress={progress.failed ? onRetry : onDone} /> : null}
    </Group>
  );
}

/** sudo's question while setup waits for your password; the answer goes straight to sudo. */
function PasswordPrompt({ prompt }: { prompt: string }) {
  const [password, setPassword] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const send = async (cancel = false) => {
    setSending(true);
    setError("");
    try {
      await api.answerSetup(cancel ? "" : password, cancel);
      setPassword("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSending(false);
    }
  };
  return (
    <form
      className="flex flex-col gap-3 bg-accent/[0.06] px-4 py-3.5"
      onSubmit={(e) => {
        e.preventDefault();
        if (password) void send();
      }}
    >
      <div className="flex items-center gap-2 text-[14px] font-medium">
        <IconTile icon={Lock} color="blue" size="sm" /> Setup needs your password
      </div>
      <div className="text-xs text-muted">
        {prompt.replace(/^\[sudo\]\s*/, "")} spillup passes it to sudo to install Tailscale and start the service at boot, and doesn’t keep it.
      </div>
      {error ? <p role="alert" className="text-xs text-danger">{error}</p> : null}
      <div className="flex gap-2">
        <input
          type="password"
          autoFocus
          aria-label="Your password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="flex-1 rounded-lg bg-surface px-3 py-1.5 text-[14px] shadow-[0_0_0_0.5px_var(--border)] outline-none focus:ring-2 focus:ring-accent/50"
        />
        <Button type="submit" size="sm" isDisabled={!password || sending}>
          Continue
        </Button>
        <Button size="sm" variant="ghost" isDisabled={sending} onPress={() => void send(true)}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

/** Links in setup's output, such as Tailscale's sign-in, open in a new tab. */
function linkify(line: string) {
  const parts = line.split(/(https?:\/\/\S+)/g);
  return parts.map((p, i) =>
    /^https?:\/\//.test(p) ? (
      <a key={i} href={p} target="_blank" rel="noopener" className="text-accent underline">
        {p}
      </a>
    ) : (
      p
    ),
  );
}
