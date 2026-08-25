[@panaudia/lasa-client](../index.md) / SinkPlayerOptions

# Interface: SinkPlayerOptions

Defined in: [src/audio/player.ts:37](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L37)

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="channelcount"></a> `channelCount?` | `number` | Channels (default 2 — binaural). | [src/audio/player.ts:41](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L41) |
| <a id="debug"></a> `debug?` | `boolean` | - | [src/audio/player.ts:70](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L70) |
| <a id="jitterconfig"></a> `jitterConfig?` | `Partial`\<[`JitterBufferCoreConfig`](JitterBufferCoreConfig.md)\> | Advanced: override v4 jitter-buffer tuning, merged over the derived geometry. | [src/audio/player.ts:62](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L62) |
| <a id="latencyhint"></a> `latencyHint?` | `AudioContextLatencyCategory` | Latency hint for the AudioContext (default 'interactive'). | [src/audio/player.ts:43](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L43) |
| <a id="qualitylevel"></a> `qualityLevel?` | `number` | LASA quality level 0–2 for this sink's playout buffer — an intent ("how latency-tolerant is this stream"), mapped to {floor, robustness, capacity} inside the core. Client-local for the sink direction (no protocol involvement); default 0 (interactive). | [src/audio/player.ts:52](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L52) |
| <a id="redundancy"></a> `redundancy?` | `number` | The entity's declared maximum redundancy offset 0–7 (lasa-core.md §4.2) — the request that the space emit repeats on this sink track. > 0 composes the FEC floor into the playout buffer (floors compose by max) and puts the worker's reassembler in front of the decoder. Default 0 (no FEC). | [src/audio/player.ts:60](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L60) |
| <a id="samplerate"></a> `sampleRate?` | `number` | Sample rate (default 48000). | [src/audio/player.ts:39](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L39) |
| <a id="statsevery"></a> `statsEvery?` | `number` | Worklet stats cadence in reads (128-sample quanta). Default 94 (~250 ms at 48 kHz). Latency harnesses shrink it (e.g. 6 ≈ 16 ms windows) to timestamp changes in the rendered output — the finer cadence costs one small postMessage per window, measurement only. | [src/audio/player.ts:69](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L69) |
| <a id="writerframesamples"></a> `writerFrameSamples?` | `number` | Writer frame (server's Opus output) in samples. Default 240 (5 ms @ 48 kHz). | [src/audio/player.ts:45](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L45) |
