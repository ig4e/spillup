// Moonlight v7 input frame. Lengths inside the input payload are network byte order.
function frame(command: number, length: number) {
  const view = new DataView(new ArrayBuffer(12 + length));
  view.setUint16(0, 0x0206, true);
  view.setUint16(2, 8 + length, true);
  view.setUint32(4, 4 + length);
  view.setUint32(8, command, true);
  return view;
}

export class DesktopInput {
  private held = new Set<number>();
  private buttons = new Set<number>();
  private write: (packet: ArrayBuffer) => void;
  private pads = new Map<number, Uint8Array>();
  constructor(write: (packet: ArrayBuffer) => void) { this.write = write; }

  gamepads(devices: (Gamepad | null)[]) {
    const standard = devices.filter((p): p is Gamepad => Boolean(p?.connected && p.mapping === "standard" && p.index < 16));
    const active = standard.reduce((mask, pad) => mask | (1 << pad.index), 0);
    const buttonFlags = [0x1000,0x2000,0x4000,0x8000,0x100,0x200,0,0,0x20,0x10,0x40,0x80,1,2,4,8,0x400];
    for (const pad of standard) {
      if (!this.pads.has(pad.index)) {
        const arrival = frame(0x55000004, 8);
        arrival.setUint8(12, pad.index);
        arrival.setUint8(13, 1); // Standard layout, Xbox controller.
        arrival.setUint16(14, pad.vibrationActuator ? 3 : 1, true);
        arrival.setUint32(16, buttonFlags.reduce((mask, b) => mask | b, 0), true);
        this.write(arrival.buffer);
      }
      const state = controllerState(pad.index, active);
      const flags = buttonFlags.reduce((mask, flag, i) => pad.buttons[i]?.pressed ? mask | flag : mask, 0);
      state.setUint16(20, flags, true);
      state.setUint8(22, Math.round(255 * (pad.buttons[6]?.value ?? 0)));
      state.setUint8(23, Math.round(255 * (pad.buttons[7]?.value ?? 0)));
      for (let i = 0; i < 4; i++) {
        const axis = Math.max(-1, Math.min(1, pad.axes[i] || 0));
        state.setInt16(24 + i * 2, Math.round(axis * (i % 2 ? -32767 : 32767)), true);
      }
      const bytes = new Uint8Array(state.buffer);
      const previous = this.pads.get(pad.index);
      if (!previous || !bytes.every((b, i) => b === previous[i])) {
        this.write(state.buffer);
        this.pads.set(pad.index, bytes);
      }
    }
    for (const id of this.pads.keys()) {
      if (!(active & (1 << id))) {
        this.write(controllerState(id, active).buffer);
        this.pads.delete(id);
      }
    }
  }

  key(key: number, down: boolean, modifiers = 0) {
    if (!key || this.held.has(key) === down) return;
    if (down) this.held.add(key);
    else this.held.delete(key);
    const view = frame(down ? 3 : 4, 6);
    view.setUint16(13, key, true);
    view.setUint8(15, modifiers);
    this.write(view.buffer);
  }

  button(button: number, down: boolean) {
    if (button < 0 || button > 4 || this.buttons.has(button) === down) return;
    if (down) this.buttons.add(button);
    else this.buttons.delete(button);
    const view = frame(down ? 8 : 9, 1);
    view.setUint8(12, button + 1);
    this.write(view.buffer);
  }

  point(x: number, y: number) {
    const view = frame(5, 10);
    view.setInt16(12, Math.round(Math.min(1, Math.max(0, x)) * 32766));
    view.setInt16(14, Math.round(Math.min(1, Math.max(0, y)) * 32766));
    view.setInt16(18, 32767);
    view.setInt16(20, 32767);
    this.write(view.buffer);
  }

  scroll(delta: number, horizontal: boolean) {
    if (!delta) return;
    const steps = Math.round(Math.max(-32767, Math.min(32767, -delta)));
    const view = frame(horizontal ? 0x55000001 : 10, horizontal ? 2 : 6);
    view.setInt16(12, steps);
    if (!horizontal) view.setInt16(14, steps);
    this.write(view.buffer);
  }

  releaseButtons() {
    for (const button of this.buttons) this.button(button, false);
  }

  release() {
    for (const key of this.held) this.key(key, false);
    this.releaseButtons();
  }

  text(value: string) {
    const bytes = new TextEncoder().encode(value);
    if (bytes.length > 64 * 1024) throw new Error("Send up to 64 KiB of text at a time.");
    this.release();
    for (let start = 0; start < bytes.length;) {
      let end = Math.min(start + 32, bytes.length);
      while (end < bytes.length && (bytes[end] & 0xc0) === 0x80) end--;
      const view = frame(0x17, end - start);
      new Uint8Array(view.buffer, 12).set(bytes.subarray(start, end));
      this.write(view.buffer);
      start = end;
    }
  }
}

const physicalModifiers: Record<string, number> = { ShiftLeft: 0xa0, ShiftRight: 0xa1, ControlLeft: 0xa2, ControlRight: 0xa3, AltLeft: 0xa4, AltRight: 0xa5, MetaLeft: 0x5b, MetaRight: 0x5c };

export function keyInput(e: KeyboardEvent) {
  return {
    key: physicalModifiers[e.code] ?? e.keyCode,
    modifiers: Number(e.shiftKey) + 2 * Number(e.ctrlKey) + 4 * Number(e.altKey) + 8 * Number(e.metaKey),
  };
}

// Pointer positions are relative to the video content after object-fit: contain.
export function screenPoint(element: HTMLElement, event: { clientX: number; clientY: number }, width: number, height: number) {
  const box = element.getBoundingClientRect();
  const scale = Math.min(box.width / width, box.height / height);
  if (!scale) return null;
  const x = (event.clientX - box.left - (box.width - width * scale) / 2) / (width * scale);
  const y = (event.clientY - box.top - (box.height - height * scale) / 2) / (height * scale);
  return { x, y };
}

// Gen 5 multi-controller state carries the controller slot and connected-slot bit mask.
function controllerState(index: number, active: number) {
  const state = frame(0x0c, 26);
  state.setUint16(12, 0x1a, true);
  state.setUint16(14, index, true);
  state.setUint16(16, active, true);
  state.setUint16(18, 0x14, true);
  state.setUint16(32, 0x9c, true);
  state.setUint16(36, 0x55, true);
  return state;
}

export function rumble(bytes: Uint8Array) {
  if (bytes.byteLength !== 14) return;
  const v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (v.getUint16(0, true) !== 0x10b) return;
  try {
    const motor = navigator.getGamepads?.()[v.getUint16(8, true)]?.vibrationActuator;
    void motor?.playEffect("dual-rumble", {
      duration: 1000,
      strongMagnitude: v.getUint16(10, true) / 65535,
      weakMagnitude: v.getUint16(12, true) / 65535,
    }).catch(() => {});
  } catch { /* Access to gamepads depends on the browser's permissions policy. */ }
}
