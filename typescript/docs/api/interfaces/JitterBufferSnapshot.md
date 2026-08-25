[@panaudia/lasa-client](../index.md) / JitterBufferSnapshot

# Interface: JitterBufferSnapshot

Defined in: [src/audio/jitter-buffer-core.ts:177](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L177)

Rich snapshot of the buffer for tuning/observability (port of Go `JitterBufferStats`).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="fillframes"></a> `fillFrames` | `number` | - | [src/audio/jitter-buffer-core.ts:178](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L178) |
| <a id="fillms"></a> `fillMs` | `number` | - | [src/audio/jitter-buffer-core.ts:179](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L179) |
| <a id="frozen"></a> `frozen` | `boolean` | Sensing + servo frozen (sustained delivery absence — outage/loss burst). | [src/audio/jitter-buffer-core.ts:192](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L192) |
| <a id="laps"></a> `laps` | `number` | Writer lapped a stalled reader. | [src/audio/jitter-buffer-core.ts:198](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L198) |
| <a id="overruns"></a> `overruns` | `number` | Near-capacity snaps (the capacity valve short of a full lap). | [src/audio/jitter-buffer-core.ts:196](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L196) |
| <a id="ratepersec"></a> `ratePerSec` | `number` | Current servo rate, frames/s of read time; + = drop trickle, − = insert. | [src/audio/jitter-buffer-core.ts:190](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L190) |
| <a id="samplesdropped"></a> `samplesDropped` | `number` | - | [src/audio/jitter-buffer-core.ts:201](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L201) |
| <a id="samplesinserted"></a> `samplesInserted` | `number` | - | [src/audio/jitter-buffer-core.ts:202](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L202) |
| <a id="setpointframes"></a> `setpointFrames` | `number` | Target pre-read fill the servo holds (F + cushion, floored, capped). | [src/audio/jitter-buffer-core.ts:181](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L181) |
| <a id="setpointms"></a> `setpointMs` | `number` | - | [src/audio/jitter-buffer-core.ts:182](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L182) |
| <a id="started"></a> `started` | `boolean` | - | [src/audio/jitter-buffer-core.ts:193](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L193) |
| <a id="trims"></a> `trims` | `number` | Macro-trims: confirmed-displacement single discards (one position jump each). | [src/audio/jitter-buffer-core.ts:200](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L200) |
| <a id="underruns"></a> `underruns` | `number` | - | [src/audio/jitter-buffer-core.ts:194](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L194) |
| <a id="widthhighframes"></a> `widthHighFrames` | `number` | Held effective bunch/reader-stall-side width (capacity currency). | [src/audio/jitter-buffer-core.ts:187](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L187) |
| <a id="widthhighms"></a> `widthHighMs` | `number` | - | [src/audio/jitter-buffer-core.ts:188](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L188) |
| <a id="widthlowframes"></a> `widthLowFrames` | `number` | Held effective late-side width (latency currency). | [src/audio/jitter-buffer-core.ts:184](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L184) |
| <a id="widthlowms"></a> `widthLowMs` | `number` | - | [src/audio/jitter-buffer-core.ts:185](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/jitter-buffer-core.ts#L185) |
