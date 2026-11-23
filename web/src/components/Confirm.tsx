import { AlertDialog, Button, Spinner } from "@heroui/react";
import { useState, type ReactNode } from "react";

export interface ConfirmRequest {
  title: string;
  body: ReactNode;
  confirm: string;
  status?: "accent" | "warning" | "danger";
  run: () => Promise<unknown>;
}

/** One dialog for every change that moves the accounts; it stays open, busy, until the change answers. */
export function useConfirm() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  const [busy, setBusy] = useState(false);

  const dialog = (
    <AlertDialog.Backdrop isOpen={!!request} onOpenChange={(open) => !open && !busy && setRequest(null)}>
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-[460px]">
          <AlertDialog.Header>
            <AlertDialog.Icon status={request?.status ?? "accent"} />
            <AlertDialog.Heading>{request?.title}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="text-sm text-muted">{request?.body}</div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button variant="tertiary" isDisabled={busy} onPress={() => setRequest(null)}>
              Cancel
            </Button>
            <Button
              variant={request?.status === "danger" ? "danger" : "primary"}
              isPending={busy}
              onPress={async () => {
                if (!request) return;
                setBusy(true);
                try {
                  await request.run();
                } finally {
                  setBusy(false);
                  setRequest(null);
                }
              }}
            >
              {({ isPending }) => (
                <>
                  {isPending ? <Spinner size="sm" color="current" /> : null}
                  {request?.confirm}
                </>
              )}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  );

  return { ask: setRequest, dialog };
}
