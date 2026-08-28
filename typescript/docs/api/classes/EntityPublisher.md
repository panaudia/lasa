[@panaudia/lasa-client](../index.md) / EntityPublisher

# Class: EntityPublisher

Defined in: [src/lasa/client.ts:135](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L135)

Publishes one entity's mono-object source track, and carries the
owner-scope base-profile operations for it (personal mutes and
solos, runtime attrs — see [EntityControls](EntityControls.md)).

## Properties

| Property | Modifier | Type | Description | Defined in |
| ------ | ------ | ------ | ------ | ------ |
| <a id="entityid"></a> `entityId` | `readonly` | `string` | The entity this publisher owns. | [src/lasa/client.ts:142](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L142) |

## Methods

### clearAttr()

```ts
clearAttr(path): Promise<void>;
```

Defined in: [src/lasa/client.ts:166](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L166)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `path` | `string` |

#### Returns

`Promise`\<`void`\>

***

### mute()

```ts
mute(other): Promise<void>;
```

Defined in: [src/lasa/client.ts:149](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L149)

Silences the pair (this entity, `other`) in both directions (profile §6).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>

***

### publish()

```ts
publish(packet): Promise<void>;
```

Defined in: [src/lasa/client.ts:184](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L184)

Sends one mono-object packet on the entity's source track. The
worker owns the track's gapless sequence (lasa-core.md §5.1: one
counter per track, shared with a running capture on this entity)
and attaches the uplink redundancy repeat at the entity's offset;
any `redundancy` on `packet` is replaced when that offset is above
0 and passed through as given when it is 0. The packet is
structured-cloned at postMessage, so its buffers are immediately
reusable. Resolves once handed to the worker, not once sent.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `packet` | [`MonoObjectPacket`](../interfaces/MonoObjectPacket.md) |

#### Returns

`Promise`\<`void`\>

***

### setAttr()

```ts
setAttr(path, value): Promise<void>;
```

Defined in: [src/lasa/client.ts:163](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L163)

Sets one runtime attribute at a dotted path.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `path` | `string` |
| `value` | [`BaseValue`](../type-aliases/BaseValue.md) |

#### Returns

`Promise`\<`void`\>

***

### setAttrs()

```ts
setAttrs(attrs): Promise<void>;
```

Defined in: [src/lasa/client.ts:170](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L170)

Sets several attributes as one group.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `attrs` | `Record`\<`string`, [`BaseValue`](../type-aliases/BaseValue.md)\> |

#### Returns

`Promise`\<`void`\>

***

### solo()

```ts
solo(other): Promise<void>;
```

Defined in: [src/lasa/client.ts:156](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L156)

Hear `other` (and the other solo'd entities) only; wins over mutes.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>

***

### unmute()

```ts
unmute(other): Promise<void>;
```

Defined in: [src/lasa/client.ts:152](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L152)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>

***

### unsolo()

```ts
unsolo(other): Promise<void>;
```

Defined in: [src/lasa/client.ts:159](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L159)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>
