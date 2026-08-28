[@panaudia/lasa-client](../index.md) / SinkAudioConfig

# Interface: SinkAudioConfig

Defined in: [src/worker/protocol.ts:30](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L30)

WebCodecs decoder config for a sink track + timestamp synthesis.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="codec"></a> `codec` | `string` | - | [src/worker/protocol.ts:31](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L31) |
| <a id="framedurationus"></a> `frameDurationUs` | `number` | Microseconds per sink frame (timestamps = seq × this). | [src/worker/protocol.ts:35](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L35) |
| <a id="numberofchannels"></a> `numberOfChannels` | `number` | - | [src/worker/protocol.ts:33](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L33) |
| <a id="samplerate"></a> `sampleRate` | `number` | - | [src/worker/protocol.ts:32](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L32) |
