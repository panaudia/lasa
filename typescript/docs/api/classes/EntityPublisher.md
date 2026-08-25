[@panaudia/lasa-client](../index.md) / EntityPublisher

# Class: EntityPublisher

Defined in: [src/lasa/client.ts:128](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L128)

Publishes one entity's mono-object source track, and carries the
owner-scope base-profile operations for it (personal mutes and
solos, runtime attrs — see [EntityControls](EntityControls.md)).

## Properties

| Property | Modifier | Type | Description | Defined in |
| ------ | ------ | ------ | ------ | ------ |
| <a id="entityid"></a> `entityId` | `readonly` | `string` | The entity this publisher owns. | [src/lasa/client.ts:138](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L138) |

## Methods

### clearAttr()

```ts
clearAttr(path): Promise<void>;
```

Defined in: [src/lasa/client.ts:162](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L162)

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

Defined in: [src/lasa/client.ts:145](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L145)

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
publish(seq, packet): Promise<void>;
```

Defined in: [src/lasa/client.ts:175](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L175)

Sends one mono-object packet with the gapless sequence number as
the datagram group id (§5.2). The framed bytes are structured-cloned
at postMessage, so the scratch is immediately reusable.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `seq` | `bigint` |
| `packet` | [`MonoObjectPacket`](../interfaces/MonoObjectPacket.md) |

#### Returns

`Promise`\<`void`\>

***

### setAttr()

```ts
setAttr(path, value): Promise<void>;
```

Defined in: [src/lasa/client.ts:159](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L159)

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

Defined in: [src/lasa/client.ts:166](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L166)

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

Defined in: [src/lasa/client.ts:152](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L152)

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

Defined in: [src/lasa/client.ts:148](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L148)

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

Defined in: [src/lasa/client.ts:155](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L155)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>
