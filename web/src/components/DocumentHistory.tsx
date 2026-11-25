import { Modal, toast } from "@heroui/react";
import { History } from "lucide-react";
import { useEffect, useState } from "react";
import { call, read } from "../lib/api";
import { useQuery } from "../lib/hooks";
import { Button } from "./Button";
import { Group, IconTile, Row } from "./ui";

interface Version { path: string; id: string; at: string }
interface Preview { before: string; diff: string; same: boolean }
const history = read<Version[]>("library/revisions", true);
const field = "block w-full min-w-0 rounded-md border border-border bg-surface px-2 py-2 text-base";

export function DocumentHistory({ onRestored }: { onRestored: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  return <>
    <Group><Row icon={<IconTile icon={History} color="indigo" />} title="Revision history"
      subtitle="Shared instructions and authored skills" onPress={() => setOpen(true)} /></Group>
    {open ? <HistoryDialog close={() => setOpen(false)} onRestored={onRestored} /> : null}
  </>;
}

function HistoryDialog({ close, onRestored }: { close: () => void; onRestored: () => Promise<void> }) {
  const versions = useQuery(history);
  const [choice, setChoice] = useState("");
  const [version, setVersion] = useState("");
  const [busy, setBusy] = useState(false);
  const [retry, setRetry] = useState(0);
  const paths = Array.from(new Set(versions.data?.map(v => v.path))).sort();
  const path = paths.includes(choice) ? choice : paths[0];
  const available = (versions.data ?? []).filter(v => v.path === path);
  const selected = available.find(v => v.at === version) ?? available[0];
  const refresh = () => { setRetry(i => i + 1); void versions.reload(); };
  return <Modal.Backdrop isOpen onOpenChange={value => { if (!value && !busy) close(); }} variant="blur">
    <Modal.Container size="md" className="max-w-full"><Modal.Dialog>
      <Modal.Header><Modal.Heading>Revision history</Modal.Heading></Modal.Header>
      <Modal.Body className="min-w-0 space-y-3">
        {versions.error ? <div role="alert"><p>{versions.error.message}</p><Button size="sm" onPress={refresh}>Try again</Button></div>
          : !versions.data ? <p role="status">Loading history…</p>
          : !selected ? <p>No shared documents have been recorded on this machine.</p> : <>
            <label className="block">Document<select disabled={busy} className={field} value={path} onChange={e => { setChoice(e.target.value); setVersion(""); }}>
              {paths.map(p => <option key={p}>{p}</option>)}
            </select></label>
            <label className="block">Recorded version<select disabled={busy} className={field} value={selected.at} onChange={e => setVersion(e.target.value)}>
              {available.map(v => <option key={v.at + v.id} value={v.at}>{new Date(v.at).toLocaleString()} · {v.id.slice(0, 8)}</option>)}
            </select></label>
            <RevisionDiff key={JSON.stringify([selected.path, selected.id, retry])} version={selected} busy={busy} setBusy={setBusy}
              refresh={refresh} done={async () => { await versions.reload(); await onRestored(); }} />
          </>}
      </Modal.Body>
      <Modal.Footer><Button variant="tertiary" isDisabled={busy} onPress={close}>Close</Button></Modal.Footer>
    </Modal.Dialog></Modal.Container>
  </Modal.Backdrop>;
}

function RevisionDiff({ version, busy, setBusy, refresh, done }: {
  version: Version; busy: boolean; setBusy: (v: boolean) => void; refresh: () => void; done: () => Promise<void>;
}) {
  const [preview, setPreview] = useState<Preview>();
  const [error, setError] = useState("");
  const { path, id } = version;
  useEffect(() => {
    let active = true;
    void call<Preview>("POST", "library/revisions/preview", { path, id }, true)
      .then(result => { if (active) setPreview(result); })
      .catch(e => { if (active) setError((e as Error).message); });
    return () => { active = false; };
  }, [path, id]);

  const restore = async () => {
    if (!preview || preview.same || busy) return;
    setBusy(true);
    try {
      await call("POST", "library/revisions/restore", { path, id, before: preview.before }, true);
      setPreview({ ...preview, before: id, same: true, diff: "" });
      toast.success("Shared document restored.");
      await done();
    } catch (e) {
      setError((e as Error).message);
    } finally { setBusy(false); }
  };
  if (error) return <div role="alert"><p>{error}</p><Button size="sm" onPress={refresh}>Refresh preview</Button></div>;
  if (!preview) return <p role="status">Loading changes…</p>;
  if (preview.same) return <p>This version matches the current document.</p>;
  return <>
    <pre aria-label="Changes to restore" tabIndex={0} className="max-h-[34dvh] max-w-full overflow-auto rounded-md bg-surface-secondary p-3 text-xs">{preview.diff || "Recreate the empty document."}</pre>
    <p className="text-sm text-muted">Restoring this file publishes a new change to the shared library.</p>
    <Button isPending={busy} onPress={() => void restore()}>Restore this version</Button>
  </>;
}
