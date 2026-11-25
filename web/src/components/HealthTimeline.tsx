import { useState } from "react";
import { Button } from "./Button";
import { Group, Row } from "./ui";
import { api } from "../lib/api";
import { useQuery } from "../lib/hooks";
import { relative } from "../lib/format";

export function HealthTimeline() {
  const [open, setOpen] = useState(false);
  return (
    <details className="mb-7" onToggle={event => setOpen(event.currentTarget.open)}>
      <summary className="cursor-pointer rounded-xl bg-surface px-4 py-3 text-[14px] font-semibold">Machine health timeline</summary>
      {open ? <Observations /> : null}
    </details>
  );
}

function Observations() {
  const history = useQuery(api.healthHistory, 30_000);
  const [machine, setMachine] = useState("");
  const [limit, setLimit] = useState(20);
  const events = history.data ?? [];
  const names = [...new Set(events.map(event => event.machine))].sort();
  const filtered = machine ? events.filter(event => event.machine === machine) : events;
  return (
    <div className="mt-3">
      {names.length > 1 ? (
        <label className="mb-3 flex items-center gap-2 px-2 text-sm">
          Machine
          <select value={machine} className="rounded-md bg-surface px-2 py-1" onChange={event => { setMachine(event.target.value); setLimit(20); }}>
            <option value="">All machines</option>
            {names.map(name => <option key={name}>{name}</option>)}
          </select>
        </label>
      ) : null}
      <Group footer="This machine keeps its last 400 observed health changes. Times show when a state was first and last observed.">
        {history.error ? <Row title="Couldn't read machine health" subtitle={history.error.message} /> : null}
        {history.loading ? <Row title="Reading health history…" /> : null}
        {history.data && !history.error && filtered.length === 0 ? <Row title="No health observations yet" subtitle="The accounts service records observations while it runs." /> : null}
        {filtered.slice(0, limit).map(event => (
          <Row key={event.since + event.machine + event.check} title={event.machine + " · " + event.check} value={event.state}
            subtitle={<><time dateTime={event.since} title={new Date(event.since).toLocaleString()}>{relative(event.since)}</time>
              {event.last !== event.since ? <> · last seen <time dateTime={event.last} title={new Date(event.last).toLocaleString()}>{relative(event.last)}</time></> : null}</>} />
        ))}
        {filtered.length > limit ? <div className="p-2"><Button size="sm" variant="secondary" onPress={() => setLimit(n => n + 20)}>Show older changes</Button></div> : null}
      </Group>
    </div>
  );
}
