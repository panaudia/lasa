[@panaudia/lasa-client](../index.md) / SinkAudioConfig

# Interface: SinkAudioConfig

Defined in: [src/worker/protocol.ts:29](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L29)

WebCodecs decoder config for a sink track + timestamp synthesis.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="codec"></a> `codec` | `string` | - | [src/worker/protocol.ts:30](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L30) |
| <a id="framedurationus"></a> `frameDurationUs` | `number` | Microseconds per sink frame (timestamps = seq × this). | [src/worker/protocol.ts:34](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L34) |
| <a id="numberofchannels"></a> `numberOfChannels` | `number` | - | [src/worker/protocol.ts:32](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L32) |
| <a id="samplerate"></a> `sampleRate` | `number` | - | [src/worker/protocol.ts:31](https://github.com/panaudia/lasa/blob/main/typescript/src/worker/protocol.ts#L31) |
