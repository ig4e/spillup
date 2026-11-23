import { Alert, Button, Spinner } from "@heroui/react";
import { CircleAlert, CircleCheck, CircleX } from "lucide-react";
import { useState } from "react";
import { Group, GroupSkeleton, LargeTitle, Mono, Row } from "../components/ui";
import { api } from "../lib/api";
import { useQuery } from "../lib/hooks";
import type { Level } from "../lib/types";

const mark: Record<Level, React.ReactNode> = {
  ok: <CircleCheck className="size-[18px] shrink-0 fill-success text-surface" />,
  warn: <CircleAlert className="size-[18px] shrink-0 fill-warning text-surface" />,
  fail: <CircleX className="size-[18px] shrink-0 fill-danger text-surface" />,
};

const command = /^(spinup|tailscale|npm|go|winget|brew|sudo) /;

export function DoctorPage() {
  const doctor = useQuery(api.doctor);
  const [rechecking, setRechecking] = useState(false);
  const data = doctor.data;
  const checking = doctor.loading || rechecking;
  const warnings = data?.sections.flatMap((s) => s.checks).filter((c) => c.level === "warn").length ?? 0;
  const verdict: Level = data?.failures ? "fail" : warnings ? "warn" : "ok";

  return (
    <>
      <LargeTitle
        title="Doctor"
        subtitle="Checks this machine, Tailscale, the accounts and your skills. Every problem comes with its fix."
        actions={
          <Button
            variant="secondary"
            size="sm"
            isPending={checking}
            onPress={async () => {
              setRechecking(true);
              await doctor.reload();
              setRechecking(false);
            }}
          >
            {({ isPending }) => (
              <>
                {isPending ? <Spinner size="sm" color="current" /> : null}
                Check Again
              </>
            )}
          </Button>
        }
      />

      {doctor.error ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>The checks couldn’t run</Alert.Title>
            <Alert.Description>{doctor.error.message}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}

      {!data ? (
        <>
          <div className="mb-7 flex flex-col items-center gap-3 rounded-2xl bg-surface py-10">
            <Spinner />
            <div className="text-[15px] text-muted">Checking…</div>
          </div>
          <GroupSkeleton rows={4} />
        </>
      ) : (
        <>
          <div className="mb-7 flex flex-col items-center rounded-2xl bg-surface px-6 py-9 text-center shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
            <span className="[&>svg]:size-14">{mark[verdict]}</span>
            <div className="mt-3 text-[22px] font-semibold tracking-tight">
              {data.failures ? `${data.failures} Problem${data.failures === 1 ? "" : "s"} Found` : warnings ? "Working, with Notes" : "Everything Looks Good"}
            </div>
            <div className="mt-1 text-[14px] text-muted">
              {data.failures ? "Run the fix under each one, then check again." : "Nothing needs your attention."}
            </div>
          </div>
          {data.sections.map((section) => (
            <Group key={section.title} title={section.title}>
              {section.checks.map((c, i) => (
                <Row
                  key={i}
                  icon={mark[c.level]}
                  title={<span className="whitespace-normal">{c.label}</span>}
                  subtitle={c.fix && !command.test(c.fix) ? <span className="whitespace-normal">{c.fix}</span> : undefined}
                  accessory={c.fix && command.test(c.fix) ? <Mono copy>{c.fix}</Mono> : undefined}
                />
              ))}
            </Group>
          ))}
        </>
      )}
    </>
  );
}
