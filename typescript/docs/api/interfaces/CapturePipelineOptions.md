[@panaudia/lasa-client](../index.md) / CapturePipelineOptions

# Interface: CapturePipelineOptions

Defined in: [src/audio/capture.ts:37](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L37)

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="channelindex"></a> `channelIndex?` | `number` | Which channel of the source feeds this (mono) pipeline. Default 0. A stereo source becomes two pipelines with indices 0 and 1; a DAW multi-channel source one pipeline per track. | [src/audio/capture.ts:43](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/capture.ts#L43) |
