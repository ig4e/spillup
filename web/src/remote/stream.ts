import { rtcReading, type StreamReading } from "./quality";
import { DesktopInput, rumble } from "./input";

const endpoint = "/remote/api/host/stream/webrtc";
export interface Connection {
  input: DesktopInput;
  transport: "WebRTC" | "WebSocket";
  readQuality: () => Promise<StreamReading>;
  close: () => void;
}
export interface StreamOptions {
  hostId: number;
  appId: number;
  width: number;
  height: number;
  video: HTMLVideoElement;
  canvas: HTMLCanvasElement;
  status: (stage: string) => void;
  failed: (error: string) => void;
  signal: AbortSignal;
  audio: AudioContext | null;
}

export function sessionOffer(sdp: string, options: Pick<StreamOptions, "hostId" | "appId" | "width" | "height">) {
  const section = sdp.indexOf("m=");
  if (section < 0) throw new Error("The browser offered no media tracks.");
  const attributes = [
    `a=x-moonlight-host-id:${options.hostId}`, `a=x-moonlight-app-id:${options.appId}`,
    `a=x-moonlight-mode:${options.width}x${options.height}x60`, "a=x-moonlight-bitrate:10000",
    "a=x-moonlight-local-audio-play-mode:0", "a=x-moonlight-hdr:0",
  ].join("\r\n");
  return sdp.slice(0, section) + attributes + "\r\n" + sdp.slice(section);
}

// WHEP returns ICE server URIs as Link headers with optional TURN credentials.
function iceServers(header: string): RTCIceServer[] {
  return [...header.matchAll(/<((?:stun|turn|turns):[^<>]+)>([^<>]*)/gi)].flatMap(([, urls, attrs]) => {
    if (!/rel="ice-server"/i.test(attrs)) return [];
    const username = /;\s*username="([^"]*)"/i.exec(attrs)?.[1];
    const credential = /;\s*credential="([^"]*)"/i.exec(attrs)?.[1];
    return [{ urls, username, credential }];
  });
}

export async function connectRTC(o: StreamOptions): Promise<Connection> {
  const timeout = AbortSignal.timeout(23_000);
  const setup = AbortSignal.any([o.signal, timeout]);
  o.status("Negotiating video and audio…");
  const config = await fetch(endpoint, { method: "OPTIONS", credentials: "same-origin", signal: setup, redirect: "error" });
  if (!config.ok) throw new Error(`Stream configuration returned HTTP ${config.status}.`);
  const peer = new RTCPeerConnection({ iceServers: iceServers(config.headers.get("Link") ?? "") });
  const media = new MediaStream();
  o.video.srcObject = media;
  const channel = peer.createDataChannel("moonlight.control.spillup", { ordered: true });
  const input = new DesktopInput((packet) => {
    if (channel.readyState !== "open") return;
    if (channel.bufferedAmount > 512 * 1024) throw new Error("The input channel is busy. Try again in a moment.");
    channel.send(packet);
  });
  let location: string | undefined;
  let closed = false;
  const pending: string[] = [];
  const cleanLocation = (value: string | null) => {
    if (!value) return undefined;
    const resource = new URL(value, window.location.origin);
    return resource.origin === window.location.origin && /^\/remote\/api\/host\/stream\/webrtc\/\d+$/.test(resource.pathname) ? resource.pathname : undefined;
  };
  const close = () => {
    if (closed) return;
    closed = true;
    o.signal.removeEventListener("abort", close);
    peer.close();
    media.getTracks().forEach((track) => track.stop());
    if (o.video.srcObject === media) o.video.srcObject = null;
    if (location) void fetch(location, { method: "DELETE", keepalive: true, credentials: "same-origin", redirect: "error", signal: AbortSignal.timeout(4000) }).catch(() => {});
  };
  o.signal.addEventListener("abort", close, { once: true });
  if (o.signal.aborted) { close(); throw o.signal.reason; }
  let sending = false;
  const flush = async () => {
    if (!location || !pending.length || sending || closed) return;
    sending = true;
    try {
      while (pending.length && !closed) {
        const body = pending.splice(0).map((c) => "a=" + c).join("\r\n");
        await fetch(location, { method: "PATCH", headers: { "Content-Type": "application/trickle-ice-sdpfrag" }, body, credentials: "same-origin", redirect: "error", signal: AbortSignal.any([o.signal, AbortSignal.timeout(5000)]) });
      }
    } catch { /* The offer also includes candidates already gathered. */ }
    finally { sending = false; }
  };
  peer.onicecandidate = ({ candidate }) => { if (candidate) { pending.push(candidate.candidate); void flush(); } };
  peer.ontrack = ({ track }) => { media.addTrack(track); void o.video.play().catch(() => {}); };
  peer.ondatachannel = ({ channel: feedback }) => {
    if (feedback.label !== "moonlight.control") return;
    feedback.binaryType = "arraybuffer";
    feedback.onmessage = ({ data }) => { if (data instanceof ArrayBuffer) rumble(new Uint8Array(data)); };
  };
  let connected = false;
  const negotiated = new Promise<void>((resolve, reject) => {
    const check = () => {
      if (peer.connectionState === "failed" || peer.connectionState === "closed") {
        if (closed) return;
        if (connected) o.failed("The desktop connection ended.");
        else reject(new Error("The direct connection could not be established."));
      } else if (!connected && channel.readyState === "open" && o.video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA && media.getVideoTracks().length) {
        connected = true;
        resolve();
      }
    };
    peer.onconnectionstatechange = check;
    channel.onopen = check;
    o.video.addEventListener("loadeddata", check, { once: true, signal: setup });
    media.onaddtrack = check;
    // addTrack on a local MediaStream does not dispatch addtrack in every browser.
    const receive = peer.ontrack;
    peer.ontrack = (event) => { receive?.call(peer, event); check(); };
    setup.addEventListener("abort", () => reject(new Error("The direct connection timed out.")), { once: true });
  });
  // Negotiation may fail while the HTTP exchange is pending.
  void negotiated.catch(() => {});
  try {
    peer.addTransceiver("video", { direction: "recvonly" });
    peer.addTransceiver("audio", { direction: "recvonly" });
    const offer = await peer.createOffer();
    await peer.setLocalDescription(offer);
    const response = await fetch(endpoint, { method: "POST", credentials: "same-origin", redirect: "error", signal: setup,
      headers: { "Content-Type": "application/sdp" }, body: sessionOffer(peer.localDescription?.sdp ?? "", o) });
    if (response.status !== 201) throw new Error(`The host declined the stream (HTTP ${response.status}).`);
    location = cleanLocation(response.headers.get("Location"));
    const answer = await response.text();
    if (answer.length > 256 * 1024) throw new Error("The relay returned an oversized session description.");
    await peer.setRemoteDescription({ type: "answer", sdp: answer });
    void flush();
    await negotiated;
    setup.throwIfAborted();
    return {
      input, close, transport: "WebRTC",
      readQuality: async () => {
        const report = await peer.getStats?.();
        return rtcReading(report, o.video.getVideoPlaybackQuality?.(), media.getVideoTracks()[0]?.id);
      },
    };
  } catch (error) {
    close();
    throw error;
  }
}
