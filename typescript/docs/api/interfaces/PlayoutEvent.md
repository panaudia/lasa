[@panaudia/lasa-client](../index.md) / PlayoutEvent

# Interface: PlayoutEvent

Defined in: [src/audio/player.ts:120](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L120)

One buffer discontinuity, timestamped at receipt on main.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="atms"></a> `atMs` | `number` | performance.now() at receipt. | [src/audio/player.ts:127](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L127) |
| <a id="fillframes"></a> `fillFrames` | `number` | - | [src/audio/player.ts:123](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L123) |
| <a id="kind"></a> `kind` | \| `"underrun"` \| `"overrun"` \| `"lap"` \| `"insert"` \| `"drop"` \| `"trim"` \| `"processor-error"` | A buffer discontinuity kind, or `processor-error` (see [SinkPlayer.getProcessorError](../classes/SinkPlayer.md#getprocessorerror)). | [src/audio/player.ts:122](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L122) |
| <a id="reads"></a> `reads` | `number` | Worklet read count at the event (2.667 ms per read at 48 kHz). | [src/audio/player.ts:125](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L125) |
