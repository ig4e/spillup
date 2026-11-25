import assert from "node:assert/strict";
import test from "node:test";
import { DesktopInput, screenPoint } from "../src/remote/input.ts";

function capture() {
  const packets = [];
  return { packets, input: new DesktopInput((packet) => packets.push(new Uint8Array(packet))) };
}

test("v7 control frames keep the outer little-endian length and inner network byte order", () => {
  const { packets, input } = capture();
  input.key(0x41, true, 2);
  input.key(0x41, true, 2);
  input.key(0x41, false);
  input.button(2, true);
  input.release();
  assert.deepEqual(Array.from(packets[0]), [6,2,14,0,0,0,0,10,3,0,0,0,0,65,0,2,0,0]);
  assert.equal(packets[1][8], 4);
  assert.deepEqual(Array.from(packets[2]), [6,2,9,0,0,0,0,5,8,0,0,0,3]);
  assert.equal(packets[3][8], 9);
  assert.equal(packets.length, 4);
  input.point(0.5, 1);
  assert.deepEqual(Array.from(packets[4].slice(12)), [63,255,127,254,0,0,127,255,127,255]);
});

test("UTF-8 paste respects the protocol's 32-byte limit without splitting a character", () => {
  const { packets, input } = capture();
  const text = "a".repeat(31) + "🌙" + "b".repeat(29);
  input.text(text);
  const parts = packets.map((p) => {
    assert.equal(p[8], 0x17);
    assert.ok(p.length <= 44);
    return new TextDecoder("utf-8", { fatal: true }).decode(p.slice(12));
  });
  assert.equal(parts.join(""), text);
  assert.throws(() => input.text("a".repeat(65537)), /64 KiB/);
});

test("standard gamepad arrival, button mask, analog axes, change suppression and disconnect", () => {
  const { packets, input } = capture();
  const buttons = Array.from({ length: 17 }, () => ({ pressed: false, value: 0 }));
  buttons[0].pressed = true;
  buttons[6].value = 0.5;
  const pad = { index: 1, mapping: "standard", connected: true, buttons, axes: [-1, -1, 0, 0] };
  input.gamepads([null, pad]); input.gamepads([null, pad]); input.gamepads([]);
  assert.equal(packets.length, 3);
  assert.deepEqual(Array.from(packets[0].slice(8,16)), [4,0,0,85,1,1,1,0]);
  assert.deepEqual(Array.from(packets[1].slice(8)), [12,0,0,0,26,0,1,0,2,0,20,0,0,16,128,0,1,128,255,127,0,0,0,0,156,0,0,0,85,0]);
  assert.equal(packets[2][16], 0);
});

test("letterbox edges stay outside the target on a portrait screen", () => {
  const area = { getBoundingClientRect: () => ({ left: 0, top: 0, width: 320, height: 600 }) };
  assert.deepEqual(screenPoint(area, {clientX:160, clientY:300}, 1920, 1080), {x:0.5, y:0.5});
  assert.ok(screenPoint(area, {clientX:160, clientY:1}, 1920, 1080).y < 0);
});
