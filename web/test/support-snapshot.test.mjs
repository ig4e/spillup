import assert from "node:assert/strict";
import test from "node:test";
import { doctorAnchor, supportSnapshot } from "../src/lib/support-snapshot.ts";

const now = new Date("2026-01-15T12:03:00Z");
const at = "2026-01-15T12:00:03Z";
const privateText = "confidential-fixture-value";
const peer = privateText + ".example.ts.net";

test("preview projects an allowlist, aliases hosts consistently and omits raw diagnostics", () => {
  const sameCheck = { id: "t3.quota-key", level: "fail", label: privateText, fix: privateText };
  const snapshot = supportSnapshot({
    now,
    doctor: { sections: [
      { title: "T3 Code", checks: [sameCheck, sameCheck] },
      { title: privateText, checks: [{ id: privateText, level: privateText, label: privateText }] },
    ], failures: 2 },
    staleChecks: true,
    overview: { name: privateText, os: "linux", hold: privateText, version: "v0.4.2", settings: { library: privateText },
      service: { version: privateText, leading: false, proxy_running: true, accounts: [{ name: privateText }] } },
    actions: [{ at, action: "mcp/" + privateText, origin: privateText, status: 401 },
      { at: privateText, action: privateText, status: privateText }],
    jobs: [{ updated: at, task: privateText, state: "interrupted", log: [privateText] }],
    health: [privateText, peer, peer, "some-other-private-host"].map(machine => ({
      machine, check: "T3 linking", state: machine === peer ? "CLI absent" : privateText, since: at,
    })),
  });
  assert.deepEqual(snapshot.health_changes.map(v => v.machine), ["this-machine", "peer-1", "peer-1", "peer-2"]);
  assert.deepEqual(snapshot.checks, [{ id: "t3.quota-key", outcome: "fail", count: 2 }, { id: "unknown", outcome: "unknown", count: 1 }]);
  assert.equal(snapshot.machine.spillup_version, "v0.4.2");
  assert.equal(snapshot.machine.accounts_service.version, "unknown");
  assert.equal(snapshot.checks_from_last_successful_refresh, true);
  assert.deepEqual(snapshot.recent_actions, [{ component: "mcp", status: 401, minutes_ago: 2 }, { component: "unknown", status: null, minutes_ago: null }]);
  assert.equal(snapshot.recent_jobs[0].outcome, "interrupted");
  assert.ok(!JSON.stringify(snapshot).includes(privateText));
  assert.ok(!JSON.stringify(snapshot).includes("some-other-private-host"));
});

test("history is bounded, missing sources differ from empty histories, and older doctor data is readable", () => {
  const missing = supportSnapshot({ now, staleChecks: false, actions: [], jobs: null, health: null });
  assert.deepEqual(missing.recent_actions, []);
  assert.equal(missing.recent_jobs, null);
  assert.equal(missing.checks, null);
  const data = supportSnapshot({ now, staleChecks: false, actions: Array(100).fill({ action: "setup", at }),
    jobs: Array(50).fill({ task: "setup", state: "failed", updated: at }),
    health: Array(100).fill({ machine: "demo", check: "Spillup", state: "Responding", since: at }),
    doctor: { sections: [{ title: "Network", checks: [{ level: "fail", label: peer }] }] },
  });
  assert.equal(data.recent_actions.length, 20);
  assert.equal(data.recent_jobs.length, 10);
  assert.equal(data.health_changes.length, 32);
  assert.deepEqual(data.checks, [{ id: "network", outcome: "fail", count: 1 }]);
  assert.equal(doctorAnchor("T3 Code"), "doctor-t3-code");
  assert.equal(doctorAnchor("Network"), "doctor-network");
  assert.equal(doctorAnchor(privateText), undefined);
});
