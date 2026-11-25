import { Modal } from "@heroui/react";
import { FileDown } from "lucide-react";
import { useState } from "react";
import { useOverview } from "../App";
import { api, ApiError } from "../lib/api";
import { supportSnapshot } from "../lib/support-snapshot";
import type { Doctor } from "../lib/types";
import { Button } from "./Button";
import { Group, IconTile, Row } from "./ui";

function optionalHistory<T>(promise: Promise<T[]>) {
  return promise.then(values => Array.isArray(values) ? values : null).catch(error => {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) throw error;
    return null;
  });
}

export function SupportSnapshot({ doctor, stale }: { doctor?: Doctor; stale: boolean }) {
  const { data: overview } = useOverview();
  const [preview, setPreview] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [unavailable, setUnavailable] = useState(false);

  const prepare = async () => {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const [actions, jobs, health] = await Promise.all([
        optionalHistory(api.recentActivity()), optionalHistory(api.jobHistory()), optionalHistory(api.healthHistory()),
      ]);
      setUnavailable(!doctor || !overview || actions === null || jobs === null || health === null);
      setPreview(JSON.stringify(supportSnapshot({ now: new Date(), overview, doctor, staleChecks: stale, actions, jobs, health }), null, 2));
    } catch (error) {
      setError(error instanceof ApiError ? "This session cannot access the snapshot." : "Couldn’t prepare a snapshot. Check the machine and try again.");
    } finally {
      setPending(false);
    }
  };

  const download = () => {
    const url = URL.createObjectURL(new Blob([preview + "\n"], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "spillup-support.json";
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  return <>
    <Group title="Support" footer={error ? <span role="alert">{error}</span> : undefined}>
      <Row icon={<IconTile icon={FileDown} color="blue" />} title="Preview support snapshot"
        subtitle="Checks, response codes and recent job outcomes." busy={pending} onPress={pending ? undefined : () => void prepare()} />
    </Group>
    {preview && <Modal.Backdrop isOpen onOpenChange={open => { if (!open) setPreview(""); }} variant="blur">
      <Modal.Container size="lg"><Modal.Dialog>
        <Modal.Header><Modal.Heading>Support snapshot</Modal.Heading></Modal.Header>
        <Modal.Body className="min-w-0 space-y-3">
          <p className="text-sm text-muted">Machine names use temporary aliases. Download this JSON to share it yourself.</p>
          {unavailable && <p className="text-sm text-muted">Some readings were unavailable. They appear as null.</p>}
          <pre aria-label="Snapshot JSON" tabIndex={0} className="max-h-[45dvh] overflow-auto whitespace-pre-wrap rounded-lg bg-surface-secondary p-3 text-xs [overflow-wrap:anywhere]">{preview}</pre>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="tertiary" onPress={() => setPreview("")}>Close</Button>
          <Button onPress={download}><FileDown className="size-4" />Download JSON</Button>
        </Modal.Footer>
      </Modal.Dialog></Modal.Container>
    </Modal.Backdrop>}
  </>;
}
