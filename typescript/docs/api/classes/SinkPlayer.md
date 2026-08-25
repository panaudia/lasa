[@panaudia/lasa-client](../index.md) / SinkPlayer

# Class: SinkPlayer

Defined in: [src/audio/player.ts:138](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L138)

Plays one sink track: the transport worker decodes into a shared
jitter ring, and an `AudioWorklet` renders it. Obtain one from
[LasaClient.playSink](LasaClient.md#playsink), or build one directly with
[SinkPlayer.create](#create) for a custom graph.

## Methods

### clearEvents()

```ts
clearEvents(): void;
```

Defined in: [src/audio/player.ts:207](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L207)

Empties the event ring.

#### Returns

`void`

***

### dispose()

```ts
dispose(): Promise<void>;
```

Defined in: [src/audio/player.ts:373](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L373)

Stops playout and closes this player's `AudioContext`. Idempotent.

#### Returns

`Promise`\<`void`\>

***

### getAudioGraphReport()

```ts
getAudioGraphReport(): AudioGraphReport;
```

Defined in: [src/audio/player.ts:341](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L341)

Channel-count-relevant graph state (stereo diagnostics).

#### Returns

[`AudioGraphReport`](../interfaces/AudioGraphReport.md)

***

### getDecoderConfig()

```ts
getDecoderConfig(frameDurationUs?): SinkAudioConfig;
```

Defined in: [src/audio/player.ts:304](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L304)

The decoder config the worker should use for this player's sink.

#### Parameters

| Parameter | Type | Default value |
| ------ | ------ | ------ |
| `frameDurationUs` | `number` | `5000` |

#### Returns

[`SinkAudioConfig`](../interfaces/SinkAudioConfig.md)

***

### getEvents()

```ts
getEvents(): PlayoutEvent[];
```

Defined in: [src/audio/player.ts:202](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L202)

Buffer discontinuities since the last [SinkPlayer.clearEvents](#clearevents), oldest first (bounded ring).

#### Returns

[`PlayoutEvent`](../interfaces/PlayoutEvent.md)[]

***

### getJitterStats()

```ts
getJitterStats(): JitterBufferSnapshot | null;
```

Defined in: [src/audio/player.ts:331](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L331)

Latest v4 jitter snapshot from the worklet, or null before the first.

#### Returns

[`JitterBufferSnapshot`](../interfaces/JitterBufferSnapshot.md) \| `null`

***

### getProcessorError()

```ts
getProcessorError(): Error | null;
```

Defined in: [src/audio/player.ts:197](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L197)

Set once the worklet processor has thrown; the player renders silence from then on.

#### Returns

`Error` \| `null`

***

### getRingHandoff()

```ts
getRingHandoff(): SinkRingHandoff;
```

Defined in: [src/audio/player.ts:294](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L294)

The shared ring + geometry for the worker's `setSinkTrack`.

#### Returns

[`SinkRingHandoff`](../interfaces/SinkRingHandoff.md)

***

### getTapB()

```ts
getTapB(): StereoMeterReport | null;
```

Defined in: [src/audio/player.ts:336](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L336)

Latest Tap B window (stereo-ness of the rendered output), or null.

#### Returns

[`StereoMeterReport`](../interfaces/StereoMeterReport.md) \| `null`

***

### getVolume()

```ts
getVolume(): number;
```

Defined in: [src/audio/player.ts:326](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L326)

Current output gain.

#### Returns

`number`

***

### resume()

```ts
resume(): Promise<void>;
```

Defined in: [src/audio/player.ts:314](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L314)

Resume the AudioContext (autoplay policies suspend it pre-gesture).

#### Returns

`Promise`\<`void`\>

***

### setStatsListener()

```ts
setStatsListener(listener): void;
```

Defined in: [src/audio/player.ts:215](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L215)

Per-window stats callback (latency harnesses; null to remove).
Fires once per worklet stats window with its receipt timestamp.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `listener` | [`SinkStatsListener`](../type-aliases/SinkStatsListener.md) \| `null` |

#### Returns

`void`

***

### setVolume()

```ts
setVolume(volume): void;
```

Defined in: [src/audio/player.ts:321](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L321)

Output gain, 0 to 1 (linear).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `volume` | `number` |

#### Returns

`void`

***

### create()

```ts
static create(options?): Promise<SinkPlayer>;
```

Defined in: [src/audio/player.ts:225](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L225)

Builds the audio graph and the shared ring. Requires cross-origin
isolation (the SDK's hard requirement) — throws otherwise. Must be
called from a user-gesture context on browsers that require one for
AudioContext.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `options` | [`SinkPlayerOptions`](../interfaces/SinkPlayerOptions.md) |

#### Returns

`Promise`\<`SinkPlayer`\>
