import { useState } from "react";
import { api, localDashboard } from "../lib/api";
import type { SetupPreview as Preview } from "../lib/types";
import { Group, Row, holdName } from "./ui";

export function SetupPreview({ name = "", role = "" }: { name?: string; role?: string }) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const load = async () => {
    setBusy(true);
    setProblem("");
    try {
      setPreview(await api.setupPreview(name, role));
    } catch (error) {
      setPreview(null);
      setProblem((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (!localDashboard) return null;

  return (
    <Group title="Setup Preview" footer={preview?.deferred}>
      <Row title={busy ? "Checking this machine…" : "Preview configuration changes"} tone="accent" busy={busy} onPress={load} />
      {problem ? <p role="alert" className="px-4 py-2 text-sm text-danger">{problem}</p> : null}
      {preview ? (
        <div aria-live="polite" className="px-4 pb-3 text-sm">
          <p className="pt-1 pb-2 font-semibold">{preview.name} · {holdName[preview.role]}</p>
          {preview.changes.length ? (
            <ul className="space-y-2">
              {preview.changes.map((change) => (
                <li key={change.action + change.target} className="break-words">
                  <span className="font-medium">{change.action} {change.target}</span>
                  <div className="text-muted">{change.detail}</div>
                </li>
              ))}
            </ul>
          ) : <p>Checked settings match this machine.</p>}
        </div>
      ) : null}
    </Group>
  );
}
