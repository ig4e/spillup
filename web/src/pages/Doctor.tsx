import { Alert, Spinner } from "@heroui/react";
import { Button } from "../components/Button";
import { CircleAlert, CircleCheck, CircleX } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Group, GroupSkeleton, LargeTitle, Row, Segmented } from "../components/ui";
import { api } from "../lib/api";
import { navigate, useAction, usePath, useQuery } from "../lib/hooks";
import { UpdatesSection } from "./Updates";
import { SupportSnapshot } from "../components/SupportSnapshot";
import { doctorAnchor } from "../lib/support-snapshot";
import type { Level } from "../lib/types";

const mark: Record<Level, React.ReactNode> = {
  ok: <CircleCheck className="size-[18px] shrink-0 fill-success text-surface" />,
  warn: <CircleAlert className="size-[18px] shrink-0 fill-warning text-surface" />,
  fail: <CircleX className="size-[18px] shrink-0 fill-danger text-surface" />,
};

// fixable is what the dashboard can run with one click; other fixes are things to do by hand.
const fixable = new Set(["spinup skills sync", "spinup update", "spinup tools", "spinup agents", "spinup packages sync", "spinup t3 setup", "spinup t3 link", "spinup restart"]);

type DoctorTab = "checks" | "updates";
const tabs: { id: DoctorTab; label: string }[] = [{ id: "checks", label: "Checks" }, { id: "updates", label: "Updates" }];

export function DoctorPage() {
  const updates = usePath() === "/updates";
  return (
    <>
      <LargeTitle title="Doctor" actions={updates ? undefined : <CheckAgain />} center={
        <Segmented label="Doctor view" value={updates ? "updates" : "checks"} options={tabs}
          onChange={(view) => navigate(view === "updates" ? "/updates" : "/doctor")} />
      } />
      {updates ? <UpdatesSection /> : <Checks />}
    </>
  );
}

function CheckAgain() {
  const doctor = useQuery(api.doctor);
  const [rechecking, setRechecking] = useState(false);
  return (
    <Button variant="secondary" size="sm" isPending={doctor.loading || rechecking} onPress={async () => {
      setRechecking(true);
      try { await doctor.reload(); } finally { setRechecking(false); }
    }}>Check Again</Button>
  );
}

function Checks() {
  const doctor = useQuery(api.doctor);
  const { pending, run } = useAction();
  const data = doctor.data;
  const lastAnchor = useRef("");
  useEffect(() => {
    const anchor = window.location.hash.slice(1);
    if (!data || !anchor || anchor === lastAnchor.current || !data.sections.some(s => doctorAnchor(s.title) === anchor)) return;
    document.getElementById(anchor)?.scrollIntoView({ block: "start" });
    lastAnchor.current = anchor;
  }, [data]);
  const warnings = data?.sections.flatMap((s) => s.checks).filter((c) => c.level === "warn").length ?? 0;
  const verdict: Level = data?.failures ? "fail" : warnings ? "warn" : "ok";

  return (
    <>
      {doctor.error ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>The checks couldn’t run</Alert.Title>
            <Alert.Description>{doctor.error.message}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}

      {!data && doctor.loading ? (
        <>
          <div className="mb-7 flex flex-col items-center gap-3 rounded-2xl bg-surface py-10">
            <Spinner />
            <div className="text-[15px] text-muted">Checking…</div>
          </div>
          <GroupSkeleton rows={4} />
        </>
      ) : data ? (
        <>
          <div className="mb-7 flex flex-col items-center rounded-2xl bg-surface px-6 py-9 text-center shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
            <span className="[&>svg]:size-14">{mark[verdict]}</span>
            <div className="mt-3 text-[22px] font-semibold tracking-tight">
              {data.failures ? `${data.failures} Problem${data.failures === 1 ? "" : "s"} Found` : warnings ? "Working, with Notes" : "Everything Looks Good"}
            </div>
            <div className="mt-1 text-[14px] text-muted">
              {doctor.refreshError ? "Showing the last completed check. Try Check Again." : data.failures ? "Run the fix under each one, then check again." : warnings ? "Review the flagged checks below." : "Nothing needs your attention."}
            </div>
          </div>
          {data.sections.map((section) => (
            <Group key={section.title} id={doctorAnchor(section.title)} className="scroll-mt-16" title={section.title}>
              {section.checks.map((c, i) => (
                <Row
                  key={i}
                  icon={mark[c.level]}
                  title={<span className="whitespace-normal">{c.label}</span>}
                  subtitle={c.fix && !fixable.has(c.fix) ? <span className="whitespace-normal">{c.fix}</span> : undefined}
                  accessory={
                    c.fix && fixable.has(c.fix) ? (
                      <Button size="sm" variant="secondary" isPending={pending === c.fix} onPress={() => run(c.fix!, () => api.fix(c.fix!), doctor.reload)}>
                        Fix
                      </Button>
                    ) : undefined
                  }
                />
              ))}
            </Group>
          ))}
        </>
      ) : null}
      {(data || doctor.error) && <SupportSnapshot doctor={data} stale={!!doctor.refreshError} />}
    </>
  );
}
