import assert from "node:assert/strict";
import test from "node:test";
import { rtcReading, streamRates } from "../src/remote/quality.ts";

test("frame and video payload rates require consecutive readings from the same source", () => {
  const first = { sampledAt: 100, source: "active", received: 4, rendered: 4, bytes: 3000 };
  assert.deepEqual(streamRates(first), { receivedFps: undefined, renderedFps: undefined, mbps: undefined });
  const second = { sampledAt: 2100, source: "active", received: 94, rendered: 86, bytes: 253000 };
  assert.deepEqual(streamRates(second, first), { receivedFps: 45, renderedFps: 41, mbps: 1 });
  assert.equal(streamRates({ ...second, received: 4 }, first).receivedFps, 0);
  for (const sample of [{ ...second, source: "new" }, { ...second, received: 2 }, { ...second, sampledAt: 90 }]) {
    assert.equal(streamRates(sample, first).receivedFps, undefined);
  }
});

test("only the video transport's selected candidate pair supplies latency, and values expose no addresses", () => {
  const report = new Map([
    ["audio", { type: "inbound-rtp", kind: "audio", bytesReceived: 200 }],
    ["retired", { type: "inbound-rtp", kind: "video", trackIdentifier: "old", bytesReceived: 900 }],
    ["video", { id: "video", type: "inbound-rtp", kind: "video", trackIdentifier: "active", timestamp: 2000, transportId: "ice", bytesReceived: 0, framesReceived: 0, framesDropped: 0 }],
    ["unused", { type: "candidate-pair", nominated: true, currentRoundTripTime: 0.5 }],
    ["ice", { type: "transport", selectedCandidatePairId: "selected" }],
    ["selected", { type: "candidate-pair", currentRoundTripTime: 0, localCandidateId: "local", remoteCandidateId: "remote" }],
    ["local", { type: "local-candidate", protocol: "udp", candidateType: "host", address: "private.example" }],
    ["remote", { type: "remote-candidate", candidateType: "relay", url: "private.example" }],
  ]);
  const reading = rtcReading(report, { totalVideoFrames: 25, droppedVideoFrames: 3 }, "active");
  assert.deepEqual(reading, { source: "video", sampledAt: 2000, received: 0, bytes: 0, rendered: 22, dropped: 0, protocol: "UDP", relay: true, rttMs: 0 });
  report.delete("ice");
  assert.equal(rtcReading(report, undefined, "active").rttMs, undefined);
});

test("missing browser counters stay unavailable while actual zeroes stay visible", () => {
  const unsupported = rtcReading(undefined, undefined);
  for (const key of ["received", "rendered", "bytes", "dropped", "rttMs", "protocol"]) assert.equal(unsupported[key], undefined);
  const playback = rtcReading(undefined, { totalVideoFrames: 0, droppedVideoFrames: 0 });
  assert.equal(playback.rendered, 0);
  assert.equal(playback.dropped, 0);
  assert.equal(rtcReading(undefined, { totalVideoFrames: 2, droppedVideoFrames: 3 }).rendered, undefined);
});
