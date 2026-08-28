[@panaudia/lasa-client](../index.md) / EntityControls

# Class: EntityControls

Defined in: [src/lasa/base.ts:206](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L206)

The operations an entity's owner may perform on it: personal mutes
and solos, and runtime attrs. Reached as methods on
[EntityPublisher](EntityPublisher.md) (`const me = await client.entity(id)`), or
constructed directly when the publisher is not needed.

Mute is bidirectional (§6): muting `other` silences the pair both
ways. Solo silences everyone but the solo'd set for this listener
only, and wins over mutes. Both are connection-scoped and reset on
reconnect — a client rewrites what it knows.

## Constructors

### Constructor

```ts
new EntityControls(writer, entityId): EntityControls;
```

Defined in: [src/lasa/base.ts:207](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L207)

#### Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `writer` | [`StateWriter`](../interfaces/StateWriter.md) | - |
| `entityId` | `string` | The owned entity these controls act for. |

#### Returns

`EntityControls`

## Properties

| Property | Modifier | Type | Description | Defined in |
| ------ | ------ | ------ | ------ | ------ |
| <a id="entityid"></a> `entityId` | `readonly` | `string` | The owned entity these controls act for. | [src/lasa/base.ts:210](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L210) |

## Methods

### clearAttr()

```ts
clearAttr(path): Promise<void>;
```

Defined in: [src/lasa/base.ts:234](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L234)

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

Defined in: [src/lasa/base.ts:216](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L216)

Silences the pair (this entity, `other`) in both directions.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>

***

### setAttr()

```ts
setAttr(path, value): Promise<void>;
```

Defined in: [src/lasa/base.ts:231](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L231)

Sets one runtime attribute, `path` dotted per §1 (`colour`, `avatar.hat`).

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

Defined in: [src/lasa/base.ts:238](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L238)

Sets several attributes as one group (flat: one entry per leaf path).

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

Defined in: [src/lasa/base.ts:223](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L223)

Hear `other` and the other solo'd entities only, overriding mutes.

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

Defined in: [src/lasa/base.ts:219](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L219)

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

Defined in: [src/lasa/base.ts:226](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L226)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `other` | `string` |

#### Returns

`Promise`\<`void`\>
