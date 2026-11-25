import { keyInput, screenPoint, type DesktopInput } from "./input";

export function bindDesktopInput(area: HTMLElement, input: DesktopInput, size: () => [number, number], escape: () => void, notify: (message: string) => void) {
  const touches = new Map<number, { x: number; y: number }>();
  let touch: { x: number; y: number; dragged: boolean; menu: boolean } | undefined;
  let hold: number | undefined;
  let point: { x: number; y: number } | null = null;
  let frame = 0;
  const handle = (fn: () => void) => { try { fn(); } catch (e) { notify((e as Error).message); } };
  const flush = () => { frame = 0; if (point) { input.point(point.x, point.y); point = null; } };
  const position = (e: PointerEvent) => screenPoint(area, e, ...size());
  const inside = (p: {x: number; y: number} | null) => p && p.x >= 0 && p.x <= 1 && p.y >= 0 && p.y <= 1;
  const move = (e: PointerEvent) => {
    if (e.pointerType === "touch") {
      const previous = touches.get(e.pointerId);
      if (!previous) return;
      touches.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (touches.size > 1) {
        handle(() => input.scroll((previous.y - e.clientY) * 2, false));
        return;
      }
      if (!touch || touch.menu) return;
      if (!touch.dragged) {
        if (Math.hypot(e.clientX - touch.x, e.clientY - touch.y) < 7) return;
        clearTimeout(hold);
        touch.dragged = true;
        handle(() => input.button(0, true));
      }
    }
    point = position(e);
    if (!area.hasPointerCapture(e.pointerId) && !inside(point)) { point = null; return; }
    if (!frame) frame = requestAnimationFrame(() => handle(flush));
  };
  const down = (e: PointerEvent) => {
    const p = position(e);
    if (!p || !inside(p) || e.button < 0) return;
    e.preventDefault();
    area.focus({ preventScroll: true });
    area.setPointerCapture(e.pointerId);
    if (e.pointerType === "touch") {
      touches.set(e.pointerId, { x: e.clientX, y: e.clientY });
      clearTimeout(hold);
      if (touches.size > 1) {
        touch = undefined;
        handle(() => input.button(0, false));
        return;
      }
      touch = { x: e.clientX, y: e.clientY, dragged: false, menu: false };
      hold = window.setTimeout(() => {
        if (!touch) return;
        touch.menu = true;
        handle(() => { input.button(2, true); input.button(2, false); });
      }, 520);
    }
    point = p;
    handle(() => { flush(); if (e.pointerType !== "touch") input.button(e.button, true); });
  };
  const up = (e: PointerEvent) => {
    if (e.pointerType === "touch") {
      if (!touches.delete(e.pointerId)) return;
      clearTimeout(hold);
      const tap = e.type === "pointerup" && touch && !touch.dragged && !touch.menu;
      touch = undefined;
      handle(() => { flush(); if (tap) input.button(0, true); input.button(0, false); });
    } else if (e.type === "pointerup") {
      handle(() => { flush(); input.button(e.button, false); });
    } else handle(() => input.releaseButtons());
  };
  const key = (e: KeyboardEvent) => {
    if (e.isComposing || (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "v") return;
    if (e.key === "Escape") escape();
    e.preventDefault();
    const k = keyInput(e);
    handle(() => input.key(k.key, e.type === "keydown", k.modifiers));
  };
  const paste = (e: ClipboardEvent) => { e.preventDefault(); handle(() => input.text(e.clipboardData?.getData("text/plain") ?? "")); };
  const scroll = (e: WheelEvent) => {
    e.preventDefault();
    const factor = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 480 : 1;
    handle(() => { input.scroll(e.deltaY * factor, false); input.scroll(e.deltaX * factor, true); });
  };
  const release = () => {
    clearTimeout(hold);
    touch = undefined; touches.clear(); point = null;
    handle(() => { input.release(); input.gamepads([]); });
  };
  let gamepadFrame = 0;
  function pollPads() {
    if (document.activeElement === area && !document.hidden) {
      try { input.gamepads(navigator.getGamepads?.() ?? []); }
      catch { /* Gamepad access may be disabled by the browser. */ }
    }
    gamepadFrame = requestAnimationFrame(pollPads);
  }
  if (window.isSecureContext && typeof navigator.getGamepads === "function") gamepadFrame = requestAnimationFrame(pollPads);
  const listeners = new AbortController();
  const opt = { signal: listeners.signal };
  area.addEventListener("pointerdown", down, opt);
  area.addEventListener("pointerup", up, opt);
  area.addEventListener("pointercancel", up, opt);
  area.addEventListener("lostpointercapture", up, opt);
  area.addEventListener("pointermove", move, opt);
  area.addEventListener("keydown", key, opt);
  area.addEventListener("keyup", key, opt);
  area.addEventListener("paste", paste, opt);
  area.addEventListener("wheel", scroll, { ...opt, passive: false });
  area.addEventListener("contextmenu", (e) => e.preventDefault(), opt);
  area.addEventListener("blur", release, opt);
  document.addEventListener("visibilitychange", () => { if (document.hidden) release(); }, opt);
  return () => { cancelAnimationFrame(frame); cancelAnimationFrame(gamepadFrame); release(); listeners.abort(); };
}
