[@panaudia/lasa-client](../index.md) / CapturePipeline

# Class: CapturePipeline

Defined in: [src/audio/capture.ts:74](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L74)

The page half of an entity's capture: a worklet writing a shared
ring the transport worker encodes from, plus the shared pose cell.
[LasaClient.startCapture](LasaClient.md#startcapture) builds one; build directly for a
custom graph.

## Properties

| Property | Modifier | Type | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="capacityframes"></a> `capacityFrames` | `readonly` | `number` | [src/audio/capture.ts:90](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L90) |

## Accessors

### sampleRate

#### Get Signature

```ts
get sampleRate(): number;
```

Defined in: [src/audio/capture.ts:191](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L191)

Sample rate of the owning context (the encoder must match it).

##### Returns

`number`

## Methods

### dispose()

```ts
dispose(): void;
```

Defined in: [src/audio/capture.ts:196](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L196)

Remove this pipeline's nodes (the caller's source/context survive).

#### Returns

`void`

***

### getHandoff()

```ts
getHandoff(): CaptureHandoff;
```

Defined in: [src/audio/capture.ts:180](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L180)

The shared cells for the worker's `setCaptureTrack`.

#### Returns

[`CaptureHandoff`](../interfaces/CaptureHandoff.md)

***

### getOverflows()

```ts
getOverflows(): number;
```

Defined in: [src/audio/capture.ts:166](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L166)

Capture-ring overflows so far (each = one dropped quantum = an upstream gap).

#### Returns

`number`

***

### setPose()

```ts
setPose(pose): void;
```

Defined in: [src/audio/capture.ts:175](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L175)

Publish the entity's freshest pose (seqlock write — wait-free, safe
at any rate: sensor callbacks, rAF, XR frames). The next framed
packet carries it.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](../interfaces/Pose.md) |

#### Returns

`void`

***

### create()

```ts
static create(source, options?): Promise<CapturePipeline>;
```

Defined in: [src/audio/capture.ts:98](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L98)

Builds the capture graph for one entity off `source` (any AudioNode
— MediaStreamAudioSourceNode for a mic, OscillatorNode in tests,
a DAW input channel…). Requires cross-origin isolation.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `source` | `AudioNode` |
| `options` | [`CapturePipelineOptions`](../interfaces/CapturePipelineOptions.md) |

#### Returns

`Promise`\<`CapturePipeline`\>
