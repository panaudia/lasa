[@panaudia/lasa-client](../index.md) / PresenceHandler

# Type Alias: PresenceHandler

```ts
type PresenceHandler = (groupId, msg) => void;
```

Defined in: [src/lasa/client.ts:51](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L51)

Receives each presence packet with its datagram group id (see [LasaClient.subscribePresence](../classes/LasaClient.md#subscribepresence)).

## Parameters

| Parameter | Type |
| ------ | ------ |
| `groupId` | `bigint` |
| `msg` | \| [`PresenceKeyframe`](../interfaces/PresenceKeyframe.md) \| [`PresenceDelta`](../interfaces/PresenceDelta.md) |

## Returns

`void`
