[@panaudia/lasa-client](../index.md) / CaptureHandle

# Interface: CaptureHandle

Defined in: [src/lasa/client.ts:80](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L80)

A running capture, returned by [LasaClient.startCapture](../classes/LasaClient.md#startcapture).

## Methods

### setPose()

```ts
setPose(pose): void;
```

Defined in: [src/lasa/client.ts:82](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L82)

Publish the entity's freshest pose — wait-free, call at any rate.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `pose` | [`Pose`](Pose.md) |

#### Returns

`void`

***

### stop()

```ts
stop(): Promise<void>;
```

Defined in: [src/lasa/client.ts:84](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L84)

Stops this entity's capture (same as [LasaClient.stopCapture](../classes/LasaClient.md#stopcapture)).

#### Returns

`Promise`\<`void`\>
