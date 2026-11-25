import assert from "node:assert/strict";
import test from "node:test";
import { QueryCache } from "../src/lib/query-cache.ts";

function deferred() {
  let resolve, reject;
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
  return { promise, resolve, reject };
}

test("a navigation joins a preload and a warm revisit reads synchronously", async () => {
  const cache = new QueryCache();
  const entry = cache.entry("machines");
  const request = deferred();
  let loads = 0;
  const load = () => { loads++; return request.promise; };
  const preload = entry.fetch(load, 15_000);
  const view = cache.entry("machines");
  assert.equal(view.fetch(load, 3000), preload);
  request.resolve({ machines: ["laptop"] });
  await preload;
  const previous = entry.snapshot();
  assert.deepEqual(previous, { data: { machines: ["laptop"] }, loading: false });
  await view.fetch(load, 15_000);
  assert.equal(view.snapshot(), previous);
  assert.equal(loads, 1);
  await view.fetch(async () => ({ machines: ["laptop"] }), 0);
  assert.equal(view.snapshot(), previous, "an unchanged poll leaves the subscribed tree alone");
});

test("a failed refresh keeps content and retries without a loading frame", async () => {
  const entry = new QueryCache().entry("skills");
  await entry.fetch(async () => ["one"], 0);
  const pending = deferred();
  const before = entry.snapshot();
  const refresh = entry.fetch(() => pending.promise, 0);
  assert.equal(entry.snapshot(), before);
  pending.reject(new Error("timeout"));
  await refresh;
  assert.deepEqual(entry.snapshot().data, ["one"]);
  assert.equal(entry.snapshot().loading, false);
  assert.equal(entry.snapshot().error, undefined);
  assert.equal(entry.snapshot().refreshError.message, "timeout");
  await entry.fetch(async () => ["two"], 0);
  assert.deepEqual(entry.snapshot(), { data: ["two"], loading: false });
});

test("changing a remote keeps each result on its own machine, even out of order", async () => {
  const cache = new QueryCache();
  const a = cache.entry("remote/a/overview");
  const b = cache.entry("remote/b/overview");
  const request = deferred();
  const old = a.fetch(() => request.promise, 0);
  await b.fetch(async () => ({ machine: "b" }), 0);
  request.resolve({ machine: "a" });
  await old;
  assert.equal(a.snapshot().data.machine, "a");
  assert.equal(b.snapshot().data.machine, "b");
  assert.equal(cache.entry("overview").snapshot().data, undefined);
});

test("a mutation revalidates observers and a pre-mutation response cannot roll data back", async () => {
  const cache = new QueryCache();
  const entry = cache.entry("skills");
  await entry.fetch(async () => ["installed"], 0);
  const old = deferred();
  const firstRead = entry.fetch(() => old.promise, 0);
  const changed = deferred();
  let refreshing;
  const stop = entry.observe(() => { refreshing = entry.fetch(() => changed.promise, 15_000); });
  cache.invalidate();
  assert.deepEqual(entry.snapshot().data, ["installed"]);
  changed.resolve([]);
  await refreshing;
  old.resolve(["installed"]);
  await firstRead;
  assert.deepEqual(entry.snapshot().data, []);
  stop();
});

test("access refusal discards cached data and late responses within the denied scope", async () => {
  const cache = new QueryCache();
  const remote = cache.entry("remote/peer/skills");
  const local = cache.entry("skills");
  await Promise.all([remote.fetch(async () => ["peer"], 0), local.fetch(async () => ["here"], 0)]);
  const old = deferred();
  const reading = remote.fetch(() => old.promise, 0);
  const error = Object.assign(new Error("access denied"), { status: 403 });
  cache.accessRefused("remote/peer/overview", error);
  old.resolve(["peer"]);
  await reading;
  assert.deepEqual(remote.snapshot(), { error, loading: false });
  assert.deepEqual(local.snapshot().data, ["here"]);
  cache.accessRefused("overview", error);
  assert.deepEqual(local.snapshot(), { error, loading: false });
  await local.fetch(async () => ["again"], 0);
  await local.fetch(async () => { throw error; }, 0);
  assert.deepEqual(local.snapshot(), { error, loading: false });
});

test("idle cache entries expire after five minutes while mounted data stays put", async t => {
  t.mock.timers.enable({ apis: ["Date"], now: 1_000 });
  const cache = new QueryCache();
  const active = cache.entry("overview");
  const unsubscribe = active.subscribe(() => {});
  const idle = cache.entry("machines");
  const load = async () => ["a"];
  await Promise.all([active.fetch(load, 0), idle.fetch(load, 0)]);
  t.mock.timers.tick(300_001);
  assert.equal(cache.entry("overview"), active);
  assert.equal(cache.entry("machines").snapshot().data, undefined);
  unsubscribe();
});


test("a capability refusal leaves unrelated views intact; a session refusal clears every machine", async () => {
  const cache = new QueryCache();
  const a = cache.entry("overview");
  const b = cache.entry("remote/peer/skills");
  await Promise.all([a.fetch(async () => "local", 0), b.fetch(async () => "peer", 0)]);
  const forbidden = Object.assign(new Error("terminals are off on this machine"), { status: 403 });
  cache.accessRefused("term", forbidden);
  cache.accessRefused("remote/peer/term", forbidden);
  assert.equal(a.snapshot().data, "local");
  assert.equal(b.snapshot().data, "peer");
  const denied = Object.assign(new Error("open the dashboard with `spinup ui` to sign in"), { status: 401 });
  cache.accessRefused("remote/peer/term", denied);
  assert.equal(a.snapshot().error, denied);
  assert.equal(b.snapshot().error, denied);
});
