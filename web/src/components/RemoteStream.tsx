import { ClipboardCopy, ClipboardPaste, Expand, Gauge, Keyboard, RotateCcw, Volume2, VolumeX, X } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../lib/api";
import { bindDesktopInput } from "../remote/events";
import { connectRTC, type Connection, type StreamOptions } from "../remote/stream";
import { connectSocket } from "../remote/socket";
import { pageHref } from "./ui";
import { streamRates, type StreamReading } from "../remote/quality";
import "./remote-stream.css";

interface Props { hostId: number; appId: number; machine: string; title: string; onEnd: () => void }

export function RemoteStream({ hostId, appId, machine, title, onEnd }: Props) {
  const root = useRef<HTMLDivElement>(null);
  const surface = useRef<HTMLDivElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const audio = useRef<AudioContext | null>(null);
  const [connection, setConnection] = useState<Connection | null>(null);
  const textInput = useRef<HTMLTextAreaElement>(null);
  const [attempt, retry] = useState(0);
  const [stage, setStage] = useState("Connecting…");
  const transport = connection?.transport;
  const [error, setError] = useState("");
  const [textMode, setTextMode] = useState<"type" | "copy" | null>(null);
  const [draft, setDraft] = useState("");
  const [notice, setNotice] = useState("");
  const [muted, setMuted] = useState(true);
  const [expanded, setExpanded] = useState(false);
  const [showQuality, setShowQuality] = useState(false);
  const qualityId = useId();
  const [reading, setReading] = useState(false);
  const live = Boolean(transport && !error);

  useEffect(() => {
    const canceled = new AbortController();
    const player = video.current!;
    const context = typeof AudioContext === "function" ? new AudioContext() : null;
    audio.current = context;
    setMuted(true);
    if (context) void context.suspend().catch(() => {});
    setError(""); setNotice(""); setConnection(null); setShowQuality(false); setStage("Connecting…");
    const failed = (reason: string) => {
      if (!canceled.signal.aborted) {
        setError(reason);
        canceled.abort();
        setConnection(null);
      }
    };
    const options: StreamOptions = {
      hostId, appId, width: 1920, height: 1080, video: player, canvas: canvas.current!,
      signal: canceled.signal, audio: context, status: setStage, failed,
    };
    async function connect() {
      let result: Connection;
      try {
        try { result = await connectRTC(options); }
        catch (direct) {
          if (canceled.signal.aborted) return;
          setStage("Trying the relay connection…");
          try { result = await connectSocket(options); }
          catch (relay) { throw new Error(`${(direct as Error).message} ${(relay as Error).message}`); }
        }
        if (canceled.signal.aborted) result.close();
        else {
          setConnection(result);
          surface.current?.focus({ preventScroll: true });
        }
      } catch (e) { failed((e as Error).message); }
    }
    void connect();
    return () => {
      canceled.abort();
      audio.current = null;
      if (context && context.state !== "closed") void context.close().catch(() => {});
    };
  }, [hostId, appId, attempt]);

  useEffect(() => {
    if (textMode) textInput.current?.focus();
  }, [textMode]);

  useEffect(() => {
    const area = surface.current;
    if (!area || !live) return;
    const input = connection?.input;
    if (!input) return;
    return bindDesktopInput(area, input, () => {
      const showingVideo = !video.current?.hidden;
      return showingVideo ? [video.current?.videoWidth || 1920, video.current?.videoHeight || 1080] : [canvas.current?.width || 1920, canvas.current?.height || 1080];
    }, () => setExpanded(false), setNotice);
  }, [live, connection]);

  const send = (text: string) => {
    if (!connection) { setNotice("The host disconnected."); return; }
    try {
      connection.input.text(text);
      setTextMode(null); setDraft(""); setNotice("Text sent.");
      surface.current?.focus();
    } catch (e) { setNotice((e as Error).message); }
  };
  const typeText = () => { setDraft(""); setTextMode("type"); setNotice(""); };
  const paste = async () => {
    if (!navigator.clipboard?.readText) { typeText(); return; }
    try { const text = await navigator.clipboard.readText(); if (text) send(text); else typeText(); }
    catch { typeText(); }
  };
  const copy = async () => {
    setReading(true); setNotice("");
    try {
      const { text } = await api.remoteClipboard(hostId);
      if (!text) { setNotice("The host clipboard is empty."); return; }
      try {
        if (!navigator.clipboard?.writeText) throw new Error();
        await navigator.clipboard.writeText(text); setNotice("Host text copied.");
      } catch { setTextMode("copy"); setDraft(text); }
    } catch (e) { setNotice((e as Error).message); }
    finally { setReading(false); }
  };
  const fullscreen = async () => {
    if (document.fullscreenElement) { await document.exitFullscreen(); return; }
    if (expanded) { setExpanded(false); return; }
    try { await root.current!.requestFullscreen(); } catch { setExpanded(true); }
  };
  const shortcut = (chord: number[]) => {
    const input = connection?.input;
    if (!input) return;
    try {
      input.release();
      const modifiers = (chord.includes(0xa2) ? 2 : 0) | (chord.includes(0xa4) ? 4 : 0);
      for (const key of chord) input.key(key, true, modifiers);
      for (const key of [...chord].reverse()) input.key(key, false);
      surface.current?.focus();
    } catch (e) { setNotice((e as Error).message); }
  };
  const toggleAudio = async () => {
    const next = !muted;
    setMuted(next);
    if (video.current) { video.current.muted = next; if (!next) await video.current.play().catch(() => {}); }
    const ctx = audio.current;
    if (ctx && ctx.state !== "closed") void (next ? ctx.suspend() : ctx.resume()).catch(() => {});
  };
  return (
    <div ref={root} className={`remote-player${expanded ? " remote-expanded" : ""}`}>
      <div className="remote-tools">
        <div className="remote-name" title={`${machine} · ${title}`}>{machine}<span> / {title}</span></div>
        <button onClick={() => void paste()} disabled={!live} aria-label="Paste text to host" title="Paste text to host"><ClipboardPaste /><span>Paste</span></button>
        <button onClick={() => void copy()} disabled={!live || reading} aria-label="Copy text from host clipboard" title="Copy text from host clipboard"><ClipboardCopy /><span>Copy</span></button>
        <button onClick={typeText} disabled={!live} aria-label="Type text" title="Type text"><Keyboard /></button>
        <button onClick={() => void toggleAudio()} aria-label={muted ? "Enable sound" : "Mute sound"} title={muted ? "Enable sound" : "Mute sound"}>{muted ? <VolumeX /> : <Volume2 />}</button>
        <button onClick={() => setShowQuality(open => !open)} disabled={!live} aria-expanded={live && showQuality} aria-controls={qualityId} aria-label="Stream quality" title="Stream quality"><Gauge /></button>
        <button onClick={() => void fullscreen()} aria-label="Toggle full screen" title="Full screen"><Expand /></button>
        <button onClick={onEnd} aria-label="Disconnect" title="Disconnect"><X /></button>
      </div>
      {live && showQuality && connection && <StreamQuality id={qualityId} connection={connection} />}
      <div className="remote-display" tabIndex={0} ref={surface} aria-label={`Remote desktop on ${machine}`}>
        <video ref={video} playsInline autoPlay muted={muted} hidden={transport === "WebSocket"} />
        <canvas ref={canvas} hidden={transport !== "WebSocket"} />
        {!live && <div className="remote-message" role="status">{error || stage}{error && <button onClick={() => retry((n) => n + 1)}><RotateCcw />Reconnect</button>}</div>}
      </div>
      {textMode && <div className="remote-text">
        <label htmlFor="remote-text-input">{textMode === "copy" ? "Host clipboard" : "Text to send"}</label>
        {textMode === "type" && <div className="remote-shortcuts" aria-label="Host shortcuts">
          <button onClick={() => shortcut([0x1b])}>Esc</button><button onClick={() => shortcut([0x09])}>Tab</button>
          <button onClick={() => shortcut([0x0d])}>Enter</button><button onClick={() => shortcut([0xa2, 0x43])}>Ctrl+C</button>
          <button onClick={() => shortcut([0xa2, 0x56])}>Ctrl+V</button><button onClick={() => shortcut([0xa4, 0x09])}>Alt+Tab</button>
        </div>}
        <textarea id="remote-text-input" ref={textInput} value={draft} readOnly={textMode === "copy"} maxLength={65536} onChange={(e) => setDraft(e.target.value)} />
        <div><button onClick={() => { setTextMode(null); setDraft(""); }}>Close</button>
          {textMode === "copy" ? <button onClick={() => { textInput.current?.focus(); textInput.current?.select(); }}>Select text</button> : <button disabled={!draft || !live} onClick={() => send(draft)}>Send text</button>}
        </div>
      </div>}
      {notice && <div role="status" className="remote-notice">{notice}<button onClick={() => setNotice("")} aria-label="Dismiss"><X /></button></div>}
    </div>
  );
}

function StreamQuality({ connection, id }: { connection: Connection; id: string }) {
  const [sample, setSample] = useState<{ reading: StreamReading; rates: ReturnType<typeof streamRates> }>();
  const [delayed, setDelayed] = useState(false);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    let previous: StreamReading | undefined;
    async function poll() {
      timer = setTimeout(() => setDelayed(true), 3500);
      try {
        const reading = await connection.readQuality();
        if (stopped) return;
        setSample({ reading, rates: streamRates(reading, previous) });
        setDelayed(false);
        previous = reading;
      } catch {
        if (!stopped) setDelayed(true);
      } finally {
        clearTimeout(timer);
        if (!stopped) timer = setTimeout(poll, 1000);
      }
    }
    void poll();
    return () => { stopped = true; clearTimeout(timer); };
  }, [connection]);
  const reading = sample?.reading;
  const value = (n?: number, unit = "", digits = 0) => n === undefined ? "—" : n.toFixed(digits) + unit;
  return <section className="remote-quality" id={id} aria-label="Stream quality">
    <div className="remote-quality-heading">
      <h2>Stream quality</h2>
      <a href={pageHref("/doctor#doctor-network")} target="_blank" rel="noopener" aria-label="Network checks in Doctor (opens in a new tab)">Network checks ↗</a>
    </div>
    <dl>
      <div><dt>Transport</dt><dd>{[connection.transport, reading?.protocol, reading?.relay && "TURN"].filter(Boolean).join(" · ")}</dd></div>
      <div><dt>Video bitrate</dt><dd>{value(sample?.rates.mbps, " Mbps", 2)}</dd></div>
      <div><dt>Received</dt><dd>{value(sample?.rates.receivedFps, " fps", 1)}</dd></div>
      <div><dt>Rendered</dt><dd>{value(sample?.rates.renderedFps, " fps", 1)}</dd></div>
      <div><dt>{connection.transport === "WebSocket" ? "Skipped before decode" : "Dropped frames"}</dt><dd>{value(reading?.dropped)}</dd></div>
      <div><dt>Peer RTT</dt><dd>{value(reading?.rttMs, " ms")}</dd></div>
    </dl>
    {delayed && <p role="status">{sample ? "Showing the last reading." : "Stream measurements are unavailable."}</p>}
  </section>;
}
