[@panaudia/lasa-client](../index.md) / Store

# Class: Store

Defined in: [src/lasa/store.ts:51](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L51)

A client state store for one subscription's scope.

## Accessors

### size

#### Get Signature

```ts
get size(): number;
```

Defined in: [src/lasa/store.ts:98](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L98)

Number of keys held.

##### Returns

`number`

## Methods

### \[iterator\]()

```ts
iterator: IterableIterator<[string, KV]>;
```

Defined in: [src/lasa/store.ts:103](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L103)

Iterates every live key.

#### Returns

`IterableIterator`\<\[`string`, [`KV`](../interfaces/KV.md)\]\>

***

### applyCatchUp()

```ts
applyCatchUp(m): Promise<void>;
```

Defined in: [src/lasa/store.ts:154](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L154)

Applies one catch-up-lane message: Begin opens the window, entries
stream lazily, and the closing Frontier finalizes — running the
snapshot deletion rule, dropping the transient records, and earning
the cursor (invariant F7).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `m` | [`StateMessage`](../type-aliases/StateMessage.md) |

#### Returns

`Promise`\<`void`\>

***

### applyLive()

```ts
applyLive(m): Promise<void>;
```

Defined in: [src/lasa/store.ts:113](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L113)

Applies one live-lane message. Live ops apply immediately under
per-key dedup; during the catch-up window every live clear —
including for keys not held — is recorded as a transient tombstone
so a late catch-up chunk cannot resurrect it (lasa-core.md §6.5).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `m` | [`StateMessage`](../type-aliases/StateMessage.md) |

#### Returns

`Promise`\<`void`\>

***

### catchUpStalled()

```ts
catchUpStalled(deadlineMs): boolean;
```

Defined in: [src/lasa/store.ts:241](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L241)

Whether a catch-up window has been open longer than deadlineMs —
the cue to unsubscribe and re-subscribe fresh rather than hold a
transient-record window forever.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `deadlineMs` | `number` |

#### Returns

`boolean`

***

### get()

```ts
get(key): Uint8Array<ArrayBufferLike> | undefined;
```

Defined in: [src/lasa/store.ts:93](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L93)

The latest value for `key`, or undefined.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `key` | `string` |

#### Returns

`Uint8Array`\<`ArrayBufferLike`\> \| `undefined`

***

### subscribeParams()

```ts
subscribeParams(): object;
```

Defined in: [src/lasa/store.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L88)

The subscribe inputs for a (re)connection: the prefix set plus the
cursor earned so far, if any.

#### Returns

`object`

| Name | Type | Defined in |
| ------ | ------ | ------ |
| `cursor?` | [`Cursor`](../interfaces/Cursor.md) | [src/lasa/store.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L88) |
| `prefixes` | `string`[] | [src/lasa/store.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L88) |

***

### terminated()

```ts
terminated(): void;
```

Defined in: [src/lasa/store.ts:228](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L228)

Informs the store its subscription instance ended (too-far-behind,
displacement, transport close). The catch-up window state —
including the transient records, which belong to the dead instance
— is discarded; eligibility resets so the next instance must earn
its own frontier before any cursor advances again.

#### Returns

`void`

***

### create()

```ts
static create(prefixes, opts?): Promise<Store>;
```

Defined in: [src/lasa/store.ts:79](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/store.ts#L79)

Creates an empty store for one prefix set. The set is
canonicalized. Async: the set-hash needs WebCrypto. A store belongs
to one space — its cursor is meaningless anywhere else.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `prefixes` | `string`[] |
| `opts` | [`StoreOptions`](../interfaces/StoreOptions.md) |

#### Returns

`Promise`\<`Store`\>
