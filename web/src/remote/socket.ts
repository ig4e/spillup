import { DesktopInput, rumble } from "./input";
import type { Connection, StreamOptions } from "./stream";

// The v3 relay frames Annex B video, Opus audio and input on one same-origin socket.
export async function connectSocket(o: StreamOptions): Promise<Connection> {
  if (!window.isSecureContext || typeof VideoDecoder === "undefined") {
    throw new Error("The relay fallback needs WebCodecs. Open Spillup on localhost or HTTPS in a browser with H.264 support.");
  }
  const videoConfig = { codec: "avc1.64002a", optimizeForLatency: true };
  if (!(await VideoDecoder.isConfigSupported(videoConfig)).supported) throw new Error("The browser cannot decode H.264 relay video.");
  o.signal.throwIfAborted();
  const address = new URL("/remote/api/host/stream/web_socket", window.location.href);
  address.protocol = address.protocol === "https:" ? "wss:" : "ws:";
  const socket = new WebSocket(address.href);
  socket.binaryType = "arraybuffer";
  const paint = o.canvas.getContext("2d", { alpha: false });
  let audio: AudioDecoder | undefined;
  let audioPts = 0;
  let packetTime = 0;
  let nextSound = 0;
  let live = false;
  let closed = false;
  let keyNeeded = true;
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const finished = new Promise<void>((ok, fail) => { resolve = ok; reject = fail; });
  void finished.catch(() => {});
  const fail = (reason: string) => {
    if (closed) return;
    if (live) o.failed(reason);
    reject(new Error(reason));
    close();
  };
  const decoder = new VideoDecoder({
    output: (frame) => {
      try {
        if (!paint || closed) return;
        if (o.canvas.width !== frame.displayWidth || o.canvas.height !== frame.displayHeight) {
          o.canvas.width = frame.displayWidth;
          o.canvas.height = frame.displayHeight;
        }
        paint.drawImage(frame, 0, 0);
        if (!live) { live = true; resolve(); }
      } finally { frame.close(); }
    },
    error: () => fail("The browser could not decode the relay video."),
  });
  const close = () => {
    if (closed) return;
    closed = true;
    clearTimeout(deadline);
    o.signal.removeEventListener("abort", cancel);
    socket.close();
    if (decoder.state !== "closed") decoder.close();
    if (audio && audio.state !== "closed") audio.close();
    reject(new Error("The stream ended."));
  };
  const cancel = () => close();
  const deadline = window.setTimeout(() => fail("The relay did not receive video from the host."), 18_000);
  o.signal.addEventListener("abort", cancel, { once: true });
  const input = new DesktopInput((frame) => {
    if (socket.readyState !== socket.OPEN) return;
    if (socket.bufferedAmount > 512 * 1024) throw new Error("The input channel is busy. Try again in a moment.");
    const message = new Uint8Array(frame.byteLength + 1);
    message.set(new Uint8Array(frame), 1);
    socket.send(message);
  });
  socket.onopen = () => socket.send(JSON.stringify({ Request: {
    host_id: o.hostId, app_id: o.appId, width: o.width, height: o.height, fps: 60, bitrate: 10_000,
    hdr: false, local_audio_play_mode: false, supported_codecs: 1, preferred_codecs: 1,
  } }));
  socket.onclose = () => fail("The relay disconnected.");
  socket.onerror = () => fail("The remote desktop relay did not answer.");
  socket.onmessage = (event: MessageEvent) => {
    try {
      if (typeof event.data === "string") {
        const response = JSON.parse(event.data).Response;
        if (!response || decoder.state !== "unconfigured") return;
        if (response.video_codec !== 1 || !paint) { fail("The relay could not negotiate H.264 video."); return; }
        decoder.configure(videoConfig);
        const context = o.audio;
        if (context && typeof AudioDecoder !== "undefined" && response.audio_channel_count > 0 && response.audio_channel_count <= 2 && response.audio_sample_rate > 0) {
          packetTime = Math.round(response.audio_samples_per_frame * 1e6 / response.audio_sample_rate);
          audio = new AudioDecoder({ error: () => {}, output: (frame) => {
            try {
              if (closed || context.state !== "running" || nextSound > context.currentTime + 0.3) return;
              const buffer = context.createBuffer(frame.numberOfChannels, frame.numberOfFrames, frame.sampleRate);
              for (let i = 0; i < frame.numberOfChannels; i++) frame.copyTo(buffer.getChannelData(i), { planeIndex: i, format: "f32-planar" });
              const source = context.createBufferSource();
              source.buffer = buffer;
              source.connect(context.destination);
              const at = Math.max(context.currentTime, nextSound);
              source.start(at);
              nextSound = at + buffer.duration;
            } finally { frame.close(); }
          } });
          audio.configure({ codec: "opus", sampleRate: response.audio_sample_rate, numberOfChannels: response.audio_channel_count });
        }
        return;
      }
      if (!(event.data instanceof ArrayBuffer) || event.data.byteLength < 2) return;
      const raw = new Uint8Array(event.data);
      if (raw[0] === 0) { rumble(raw.subarray(1)); return; }
      if (raw[0] === 1 && raw.length > 6 && decoder.state === "configured") {
        const key = raw[1] === 1;
        if (decoder.decodeQueueSize > 8 && !keyNeeded) {
          keyNeeded = true;
          socket.send(Uint8Array.of(0, 2, 3, 2, 0, 0, 0)); // Request a fresh key frame.
        }
        if (keyNeeded && !key) return;
        keyNeeded = false;
        decoder.decode(new EncodedVideoChunk({ type: key ? "key" : "delta", timestamp: new DataView(raw.buffer).getUint32(2), data: raw.subarray(6) }));
      } else if (raw[0] === 2 && audio?.state === "configured") {
        if (audio.decodeQueueSize < 12) audio.decode(new EncodedAudioChunk({ type: "key", timestamp: audioPts, data: raw.subarray(1) }));
        audioPts += packetTime;
      }
    } catch { fail("The relay returned an unreadable stream."); }
  };
  await finished;
  clearTimeout(deadline);
  o.ready("WebSocket");
  return { input, close };
}
