[@panaudia/lasa-client](../index.md) / CaptureHandle

# Interface: CaptureHandle

Defined in: [src/lasa/client.ts:73](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L73)

A running capture, returned by [LasaClient.startCapture](../classes/LasaClient.md#startcapture).

## Methods

### setPose()

```ts
setPose(pose): void;
```

Defined in: [src/lasa/client.ts:75](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L75)

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

Defined in: [src/lasa/client.ts:77](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L77)

Stops this entity's capture (same as [LasaClient.stopCapture](../classes/LasaClient.md#stopcapture)).

#### Returns

`Promise`\<`void`\>
