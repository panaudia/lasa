[@panaudia/lasa-client](../index.md) / BaseView

# Class: BaseView

Defined in: [src/lasa/base.ts:428](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L428)

A typed read view over a [Store](Store.md) whose prefixes include
`lasa.`. Every call scans the store, which is the right shape for a
few hundred keys; an application with more keeps its own index from
`store` iteration. Read after `await sync.settled()`.

Keys under an entity whose existence marker is absent are ignored,
as the profile requires (§6) — a departing entity's subtree can be
mid-sweep.

## Constructors

### Constructor

```ts
new BaseView(store): BaseView;
```

Defined in: [src/lasa/base.ts:429](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L429)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `store` | [`Store`](Store.md) |

#### Returns

`BaseView`

## Methods

### blockedClients()

```ts
blockedClients(): BlockView[];
```

Defined in: [src/lasa/base.ts:477](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L477)

Every blocked client.

#### Returns

[`BlockView`](../interfaces/BlockView.md)[]

***

### blockedRoles()

```ts
blockedRoles(): BlockView[];
```

Defined in: [src/lasa/base.ts:480](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L480)

#### Returns

[`BlockView`](../interfaces/BlockView.md)[]

***

### channel()

```ts
channel(channel): object;
```

Defined in: [src/lasa/base.ts:485](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L485)

Channel policy: muted flag and any set gain/attenuation override.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `channel` | `string` |

#### Returns

`object`

| Name | Type | Defined in |
| ------ | ------ | ------ |
| `attenuation?` | `number` | [src/lasa/base.ts:485](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L485) |
| `gain?` | `number` | [src/lasa/base.ts:485](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L485) |
| `muted` | `boolean` | [src/lasa/base.ts:485](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L485) |

***

### channels()

```ts
channels(): string[];
```

Defined in: [src/lasa/base.ts:496](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L496)

Every channel named by a live entity's membership or by channel policy, sorted.

#### Returns

`string`[]

***

### clientBlock()

```ts
clientBlock(clientId): BlockView | undefined;
```

Defined in: [src/lasa/base.ts:470](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L470)

The client's block, or undefined when not blocked. Expiry is not evaluated here — the server's clock decides.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `clientId` | `string` |

#### Returns

[`BlockView`](../interfaces/BlockView.md) \| `undefined`

***

### entities()

```ts
entities(): EntityView[];
```

Defined in: [src/lasa/base.ts:432](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L432)

Every live entity (marker present), in key order.

#### Returns

[`EntityView`](../interfaces/EntityView.md)[]

***

### entity()

```ts
entity(id): EntityView | undefined;
```

Defined in: [src/lasa/base.ts:453](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L453)

One entity, or undefined if its marker is absent.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `id` | `string` |

#### Returns

[`EntityView`](../interfaces/EntityView.md) \| `undefined`

***

### isEntityMuted()

```ts
isEntityMuted(entityId): boolean;
```

Defined in: [src/lasa/base.ts:465](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L465)

Whether the space has muted the entity's source (§5).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`boolean`

***

### roleBlock()

```ts
roleBlock(role): BlockView | undefined;
```

Defined in: [src/lasa/base.ts:473](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/base.ts#L473)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `role` | `string` |

#### Returns

[`BlockView`](../interfaces/BlockView.md) \| `undefined`
