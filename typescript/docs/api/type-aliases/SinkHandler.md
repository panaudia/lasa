[@panaudia/lasa-client](../index.md) / SinkHandler

# Type Alias: SinkHandler

```ts
type SinkHandler = (seq, packet) => void;
```

Defined in: [src/lasa/client.ts:53](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L53)

Receives each sink packet with its sequence number (see [LasaClient.subscribeSink](../classes/LasaClient.md#subscribesink)).

## Parameters

| Parameter | Type |
| ------ | ------ |
| `seq` | `bigint` |
| `packet` | [`SinkPacket`](../interfaces/SinkPacket.md) |

## Returns

`void`
