[@panaudia/lasa-client](../index.md) / JitterBufferCoreConfig

# Interface: JitterBufferCoreConfig

Defined in: [src/audio/jitter-buffer-core.ts:63](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L63)

Configuration for `JitterBufferCore`. All frame/latency fields are
in **samples** (frames) — the browser knows these concretely: R = the
128-sample worklet quantum, W = the server's 240-sample Opus frame. This
is the Go config's exact-count form (`WriterFrames`/`ReaderFrames`, added
in TS-port phase 0 precisely because durations cannot express the
128-sample quantum); the duration-based fields do not exist here.

The per-buffer surface is {floor, robustness, capacity} plus geometry
(design §8) — no per-route presets. `qualityLevel` maps the LASA stream
quality intent (0–2) onto those three, exactly Go's `QualityLevel`;
explicit fields override the level's values.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="capacity"></a> `capacity?` | `number` | Ring/latency budget, frames; 0/omitted derives the worst case. Overrides the level's capacity. | [src/audio/jitter-buffer-core.ts:95](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L95) |
| <a id="floor"></a> `floor?` | `number` | Declared minimum operating latency, samples — e.g. the FEC floor (`fecFloor`). Composes with the quality level's floor and the width-derived setpoint by MAX (floors compose; they never stack — mirrors Go `engine.AddSource`). | [src/audio/jitter-buffer-core.ts:87](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L87) |
| <a id="numchannels"></a> `numChannels?` | `number` | Channels per frame (interleaved). Default 1. Binaural playout is 2. | [src/audio/jitter-buffer-core.ts:67](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L67) |
| <a id="qualitylevel"></a> `qualityLevel?` | `number` | LASA quality level 0–2 (an INTENT — "how latency-tolerant is this stream"). Maps to {floor, robustness, capacity} defaults: 0 = interactive (0, 1, derived), 1 = resilient (50 ms, 4, 500 ms), 2 = playback/broadcast (150 ms, 8, 400 ms). Default 0. | [src/audio/jitter-buffer-core.ts:80](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L80) |
| <a id="readerframe"></a> `readerFrame?` | `number` | R — reader frame, samples. Required: the caller declares its callback size (browser worklet: 128). | [src/audio/jitter-buffer-core.ts:69](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L69) |
| <a id="robustness"></a> `robustness?` | `number` | The latency↔robustness knob κ ≥ 1 (values < 1 clamp to 1, the low-latency end). Scales responses to MEASURED spread, so it costs nothing on clean networks. Overrides the level's κ. | [src/audio/jitter-buffer-core.ts:93](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L93) |
| <a id="safety"></a> `safety?` | `number` | S — static pad above the underrun edge, samples. Default = 1 ms. | [src/audio/jitter-buffer-core.ts:73](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L73) |
| <a id="samplerate"></a> `sampleRate?` | `number` | Sample rate (Hz). Used for ms-derived tuning constants and stats. Default 48000. | [src/audio/jitter-buffer-core.ts:65](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L65) |
| <a id="sharedstorage"></a> `sharedStorage?` | `Float32Array`\<`ArrayBufferLike`\> | Cross-thread mode. When the writer (decode Worker) and the reader (worklet) are different threads, the ring storage and writePos must live in shared memory. Provide a `SharedArrayBuffer`-backed interleaved `Float32Array` of exactly `capacity*nc` and a length-1 `BigInt64Array` for the (atomic) writePos; `readPos` stays reader-owned (plain). Size them with `computeJitterCapacity`. Omit both for single-thread mode. | [src/audio/jitter-buffer-core.ts:104](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L104) |
| <a id="sharedwritepos"></a> `sharedWritePos?` | `BigInt64Array`\<`ArrayBufferLike`\> | SAB-backed length-1 BigInt64Array holding the atomic cumulative writePos (frames). | [src/audio/jitter-buffer-core.ts:106](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L106) |
| <a id="writerframe"></a> `writerFrame?` | `number` | W — writer frame, samples. Required: the caller declares its wire framing (server Opus: 240). | [src/audio/jitter-buffer-core.ts:71](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L71) |
