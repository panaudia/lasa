[@panaudia/lasa-client](../index.md) / SinkRingHandoff

# Interface: SinkRingHandoff

Defined in: [src/audio/player.ts:74](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L74)

What the worker needs to write this player's ring.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="jbufconfig"></a> `jbufConfig` | [`JitterBufferCoreConfig`](JitterBufferCoreConfig.md) | - | [src/audio/player.ts:75](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L75) |
| <a id="redundancy"></a> `redundancy` | `number` | Declared max redundancy offset — the worker's reassembler switch. | [src/audio/player.ts:79](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L79) |
| <a id="sharedstorage"></a> `sharedStorage` | `Float32Array` | - | [src/audio/player.ts:76](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L76) |
| <a id="sharedwritepos"></a> `sharedWritePos` | `BigInt64Array` | - | [src/audio/player.ts:77](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L77) |
