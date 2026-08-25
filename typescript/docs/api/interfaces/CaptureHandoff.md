[@panaudia/lasa-client](../index.md) / CaptureHandoff

# Interface: CaptureHandoff

Defined in: [src/audio/capture.ts:26](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L26)

Everything the worker needs to run this pipeline's encode half.

## Properties

| Property | Type | Defined in |
| ------ | ------ | ------ |
| <a id="capacityframes"></a> `capacityFrames` | `number` | [src/audio/capture.ts:28](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L28) |
| <a id="numchannels"></a> `numChannels` | `number` | [src/audio/capture.ts:27](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L27) |
| <a id="poseseq"></a> `poseSeq` | `Int32Array` | [src/audio/capture.ts:33](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L33) |
| <a id="posevalues"></a> `poseValues` | `Float64Array` | [src/audio/capture.ts:34](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L34) |
| <a id="sharedreadpos"></a> `sharedReadPos` | `BigInt64Array` | [src/audio/capture.ts:31](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L31) |
| <a id="sharedsignal"></a> `sharedSignal` | `Int32Array` | [src/audio/capture.ts:32](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L32) |
| <a id="sharedstorage"></a> `sharedStorage` | `Float32Array` | [src/audio/capture.ts:29](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L29) |
| <a id="sharedwritepos"></a> `sharedWritePos` | `BigInt64Array` | [src/audio/capture.ts:30](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L30) |
