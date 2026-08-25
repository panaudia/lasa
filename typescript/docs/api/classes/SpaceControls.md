[@panaudia/lasa-client](../index.md) / SpaceControls

# Class: SpaceControls

Defined in: src/lasa/base.ts:258

Space-wide operations, reached as `client.space`. Each is gated by
role on the server (`moderator` for entity mutes and client blocks,
`producer` for channels, rendering and role blocks — profile §2) and
silently dropped when the caller lacks it.

Grouped by concern: **hearing** is channel membership and channel
policy; **moderation** is blocks, which act on the connection; the
space entity mute is the one moderation act on hearing.

## Constructors

### Constructor

```ts
new SpaceControls(writer): SpaceControls;
```

Defined in: src/lasa/base.ts:259

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `writer` | [`StateWriter`](../interfaces/StateWriter.md) |

#### Returns

`SpaceControls`

## Methods

### addHeardIn()

```ts
addHeardIn(entityId, channel): Promise<void>;
```

Defined in: src/lasa/base.ts:320

Adds the entity's source to a channel (it is heard there).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### addHears()

```ts
addHears(entityId, channel): Promise<void>;
```

Defined in: src/lasa/base.ts:327

Adds the entity's sink to a channel (it hears there).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### blockClient()

```ts
blockClient(clientId, until?): Promise<void>;
```

Defined in: src/lasa/base.ts:275

Terminates the client's live connection and refuses readmission
until `until` (forever when omitted). Admin is unblockable.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `clientId` | `string` |
| `until?` | `Date` |

#### Returns

`Promise`\<`void`\>

***

### blockRole()

```ts
blockRole(role, until?): Promise<void>;
```

Defined in: src/lasa/base.ts:288

Blocks every connection bearing the role (except admin).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `role` | `string` |
| `until?` | `Date` |

#### Returns

`Promise`\<`void`\>

***

### kickClient()

```ts
kickClient(clientId, forMs): Promise<void>;
```

Defined in: src/lasa/base.ts:282

A block with a short expiry — "a kick is a block with a short expiry" (§5).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `clientId` | `string` |
| `forMs` | `number` |

#### Returns

`Promise`\<`void`\>

***

### muteChannel()

```ts
muteChannel(channel): Promise<void>;
```

Defined in: src/lasa/base.ts:298

A muted channel contributes nothing to audibility.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### muteEntity()

```ts
muteEntity(entityId): Promise<void>;
```

Defined in: src/lasa/base.ts:264

Silences the entity's source in every render. Rendering only; the connection stays.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`Promise`\<`void`\>

***

### removeHeardIn()

```ts
removeHeardIn(entityId, channel): Promise<void>;
```

Defined in: src/lasa/base.ts:323

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### removeHears()

```ts
removeHears(entityId, channel): Promise<void>;
```

Defined in: src/lasa/base.ts:330

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### setChannelAttenuation()

```ts
setChannelAttenuation(channel, attenuation): Promise<void>;
```

Defined in: src/lasa/base.ts:310

Channel attenuation 0–3.0; the lowest set value across a pair's shared channels overrides the source's. `undefined` clears.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `channel` | `string` |
| `attenuation` | `number` \| `undefined` |

#### Returns

`Promise`\<`void`\>

***

### setChannelGain()

```ts
setChannelGain(channel, gain): Promise<void>;
```

Defined in: src/lasa/base.ts:305

Channel gain 0–3.0 (amplitude); the highest across a pair's shared channels wins. `undefined` clears.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `channel` | `string` |
| `gain` | `number` \| `undefined` |

#### Returns

`Promise`\<`void`\>

***

### setChannels()

```ts
setChannels(
   entityId, 
   current, 
next): Promise<void>;
```

Defined in: src/lasa/base.ts:339

Replaces the entity's memberships as one group: `current` is what
the store holds now (see [BaseView.entity](BaseView.md#entity)), `next` the wanted
sets. Omitted sides are left alone. Emptying `heardIn` leaves the
entity inaudible (§6) — a moderation act, so it is allowed.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `current` | \{ `heardIn`: readonly `string`[]; `hears`: readonly `string`[]; \} |
| `current.heardIn` | readonly `string`[] |
| `current.hears` | readonly `string`[] |
| `next` | \{ `heardIn?`: readonly `string`[]; `hears?`: readonly `string`[]; \} |
| `next.heardIn?` | readonly `string`[] |
| `next.hears?` | readonly `string`[] |

#### Returns

`Promise`\<`void`\>

***

### setDof()

```ts
setDof(entityId, dof): Promise<void>;
```

Defined in: src/lasa/base.ts:377

Degrees of freedom the space enforces at ingest: 6 free, 3 turn in place, 0 fixed (`moderator`).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `dof` | `0` \| `3` \| `6` |

#### Returns

`Promise`\<`void`\>

***

### setPriority()

```ts
setPriority(entityId, priority): Promise<void>;
```

Defined in: src/lasa/base.ts:371

Marks the entity's source as important to render well — sparingly (§6).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `priority` | `boolean` |

#### Returns

`Promise`\<`void`\>

***

### setRender()

```ts
setRender(entityId, params): Promise<void>;
```

Defined in: src/lasa/base.ts:357

Sets the given render parameters as one group; `undefined` fields are untouched, `null` clears to the default.

#### Parameters

| Parameter | Type | Description |
| ------ | ------ | ------ |
| `entityId` | `string` | - |
| `params` | \{ `attenuation?`: `number` \| `null`; `directivity?`: `number` \| `null`; `gain?`: `number` \| `null`; `size?`: `number` \| `null`; \} | - |
| `params.attenuation?` | `number` \| `null` | Distance-attenuation exponent 0.0–3.0; absent = 2.0 (inverse-square). |
| `params.directivity?` | `number` \| `null` | 0.0 omnidirectional – 1.0 maximally directional. |
| `params.gain?` | `number` \| `null` | Amplitude multiplier 0.0–3.0; absent = 1.0. |
| `params.size?` | `number` \| `null` | Source size in metres, ≥ 0; absent = point source. |

#### Returns

`Promise`\<`void`\>

***

### unblockClient()

```ts
unblockClient(clientId): Promise<void>;
```

Defined in: src/lasa/base.ts:278

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `clientId` | `string` |

#### Returns

`Promise`\<`void`\>

***

### unblockRole()

```ts
unblockRole(role): Promise<void>;
```

Defined in: src/lasa/base.ts:291

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `role` | `string` |

#### Returns

`Promise`\<`void`\>

***

### unmuteChannel()

```ts
unmuteChannel(channel): Promise<void>;
```

Defined in: src/lasa/base.ts:301

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `channel` | `string` |

#### Returns

`Promise`\<`void`\>

***

### unmuteEntity()

```ts
unmuteEntity(entityId): Promise<void>;
```

Defined in: src/lasa/base.ts:267

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`Promise`\<`void`\>
