import test from "node:test";
import assert from "node:assert/strict";
import { activityItems } from "../src/lib/activity.ts";

test("only explicit job IDs link requests, including background failures after HTTP success", () => {
  const at = "2026-01-01T10:00:00Z";
  const base = { at, action: "update/system", origin: "local dashboard", status: 200, elapsed_ms: 20 };
  const job = { started: at, updated: at, task: "update", state: "failed" };
  const items = activityItems([{...base,id:"first"}, {...base,id:"second"}, base], [{...job,action_id:"second"},job]);
  assert.equal(items.length, 4);
  assert.equal(items.find(row => row.key === "first").outcome, "Accepted");
  const failed = items.find(row => row.key === "second");
  assert.equal(failed.outcome, "Failed");
  assert.equal(failed.job.action_id, "second");
  assert.equal(failed.area.doctor, "spinup");
  assert.equal(items.filter(row => row.outcome === "Failed").length, 2);
});
