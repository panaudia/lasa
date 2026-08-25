[@panaudia/lasa-client](../index.md) / AudioGraphReport

# Interface: AudioGraphReport

Defined in: [src/audio/player.ts:86](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L86)

Snapshot of the playout graph's channel-count-relevant state (the old
SDK's stereo-diagnostics report, trimmed to this topology).

## Properties

| Property | Type | Defined in |
| ------ | ------ | ------ |
| <a id="context"></a> `context` | `object` | [src/audio/player.ts:87](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L87) |
| `context.baseLatencyMs` | `number` \| `null` | [src/audio/player.ts:90](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L90) |
| `context.outputLatencyMs` | `number` \| `null` | [src/audio/player.ts:91](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L91) |
| `context.sampleRate` | `number` | [src/audio/player.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L88) |
| `context.state` | `string` | [src/audio/player.ts:89](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L89) |
| <a id="destination"></a> `destination` | `object` | [src/audio/player.ts:93](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L93) |
| `destination.channelCount` | `number` | [src/audio/player.ts:94](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L94) |
| `destination.channelCountMode` | `string` | [src/audio/player.ts:96](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L96) |
| `destination.channelInterpretation` | `string` | [src/audio/player.ts:97](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L97) |
| `destination.maxChannelCount` | `number` | [src/audio/player.ts:95](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L95) |
| <a id="ring"></a> `ring` | `object` | [src/audio/player.ts:105](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L105) |
| `ring.capacityFrames` | `number` | [src/audio/player.ts:108](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L108) |
| `ring.numChannels` | `number` | [src/audio/player.ts:107](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L107) |
| `ring.writerFrameSamples` | `number` | [src/audio/player.ts:106](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L106) |
| <a id="worklet"></a> `worklet` | `object` | [src/audio/player.ts:99](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L99) |
| `worklet.channelCount` | `number` | [src/audio/player.ts:101](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L101) |
| `worklet.channelCountMode` | `string` | [src/audio/player.ts:102](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L102) |
| `worklet.channelInterpretation` | `string` | [src/audio/player.ts:103](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L103) |
| `worklet.outputChannelCount` | `number` | [src/audio/player.ts:100](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L100) |
