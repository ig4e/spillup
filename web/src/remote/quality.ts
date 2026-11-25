export interface VideoCounters {
  received?: number;
  rendered?: number;
  bytes?: number;
  dropped?: number;
}

export interface StreamReading extends VideoCounters {
  source?: string;
  sampledAt: number;
  protocol?: "TCP" | "UDP";
  relay?: boolean;
  rttMs?: number;
}

export function streamRates(current: StreamReading, previous?: StreamReading) {
  const elapsed = previous && current.source === previous.source ? (current.sampledAt - previous.sampledAt) / 1000 : 0;
  const perSecond = (now?: number, then?: number) =>
    elapsed > 0 && now !== undefined && then !== undefined && now >= then ? (now - then) / elapsed : undefined;
  const bytesPerSecond = perSecond(current.bytes, previous?.bytes);
  return {
    receivedFps: perSecond(current.received, previous?.received),
    renderedFps: perSecond(current.rendered, previous?.rendered),
    mbps: bytesPerSecond === undefined ? undefined : bytesPerSecond * 8 / 1_000_000,
  };
}

const counter = (value: unknown) => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;

export function rtcReading(report: RTCStatsReport | undefined, playback: VideoPlaybackQuality | undefined, trackId?: string): StreamReading {
  // The stream negotiates one video receiver. Ignore audio and reports for retired tracks.
  const video: RTCInboundRtpStreamStats | undefined = report && Array.from(report.values()).find(row =>
    row.type === "inbound-rtp" && row.kind === "video" && (!trackId || !row.trackIdentifier || row.trackIdentifier === trackId));
  const transport = video?.transportId && report?.get(video.transportId);
  const pair = transport?.selectedCandidatePairId && report?.get(transport.selectedCandidatePairId);
  const local = pair && report?.get(pair.localCandidateId);
  const remote = pair && report?.get(pair.remoteCandidateId);
  const rtt = counter(pair?.currentRoundTripTime);
  const total = counter(playback?.totalVideoFrames);
  const dropped = counter(playback?.droppedVideoFrames);
  const displayed = total !== undefined && dropped !== undefined && total >= dropped ? total - dropped : undefined;
  return {
    source: video?.id, sampledAt: counter(video?.timestamp) ?? performance.now(),
    received: counter(video?.framesReceived), bytes: counter(video?.bytesReceived),
    rendered: counter(video?.framesRendered) ?? displayed, dropped: counter(video?.framesDropped) ?? dropped,
    protocol: local?.protocol === "udp" ? "UDP" : local?.protocol === "tcp" ? "TCP" : undefined,
    relay: local?.candidateType === "relay" || remote?.candidateType === "relay", rttMs: rtt === undefined ? undefined : rtt * 1000,
  };
}
