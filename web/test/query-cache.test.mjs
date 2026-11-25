import assert from "node:assert/strict";
import test from "node:test";
import { createDashboardClient, clearDeniedData, dashboardKey } from "../src/lib/query-cache.ts";

test("navigation shares a preload, and failed refresh preserves the last reading", async () => {
  const client = createDashboardClient();
  const key = dashboardKey("machines/managed");
  let read, requests = 0;
  const options = { queryKey: key, queryFn: () => { requests++; return new Promise(done => { read = done; }); } };
  const warm = client.prefetchQuery(options);
  const visit = client.fetchQuery(options);
  read(["desk"]);
  await Promise.all([warm, visit]);
  const original = client.getQueryData(key);
  await client.fetchQuery(options);
  assert.equal(requests, 1);
  await assert.rejects(client.fetchQuery({queryKey: key, staleTime: 0, queryFn: () => Promise.reject(Error("offline"))}));
  assert.deepEqual(original, ["desk"]);
  assert.equal(client.getQueryData(key), original);
  client.clear();
});

test("losing a peer session clears its reads and rejects their late completions", async () => {
  const client = createDashboardClient();
  const local = dashboardKey("skills"), remote = dashboardKey("remote/desk/skills");
  client.setQueryData(local, ["local"]);
  client.setQueryData(remote, ["original"]);
  let finish;
  const late = client.prefetchQuery({ queryKey: remote, staleTime: 0, queryFn: () => new Promise(resolve => { finish = resolve; }) });
  const denied = Object.assign(Error("missing or wrong key for this machine's dashboard"), {status: 403});
  clearDeniedData(client, "remote/desk/overview", denied);
  finish(["late"]);
  await late;
  assert.equal(client.getQueryData(remote), undefined);
  assert.equal(client.getQueryState(remote).error, denied);
  assert.deepEqual(client.getQueryData(local), ["local"]);
  client.clear();
});

test("terminal permission is scoped while a revoked dashboard session clears all machines", () => {
  const client = createDashboardClient();
  const local = dashboardKey("overview"), peer = dashboardKey("remote/peer/skills");
  client.setQueryData(local, "here");
  client.setQueryData(peer, "peer");
  clearDeniedData(client, "remote/peer/term", Object.assign(Error("Terminals are off"), {status: 403}));
  assert.equal(client.getQueryData(peer), "peer");
  const expired = Object.assign(Error("open the dashboard with `spinup ui` to sign in"), {status: 401});
  clearDeniedData(client, "remote/peer/term", expired);
  assert.equal(client.getQueryData(local), undefined);
  assert.equal(client.getQueryData(peer), undefined);
  client.clear();
});
