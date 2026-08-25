[@panaudia/lasa-client](../index.md) / SinkStatsListener

# Type Alias: SinkStatsListener

```ts
type SinkStatsListener = (snapshot, tapB, atMs) => void;
```

Defined in: [src/audio/player.ts:112](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L112)

## Parameters

| Parameter | Type |
| ------ | ------ |
| `snapshot` | [`JitterBufferSnapshot`](../interfaces/JitterBufferSnapshot.md) |
| `tapB` | [`StereoMeterReport`](../interfaces/StereoMeterReport.md) \| `null` |
| `atMs` | `number` |

## Returns

`void`
