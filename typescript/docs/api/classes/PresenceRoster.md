[@panaudia/lasa-client](../index.md) / PresenceRoster

# Class: PresenceRoster

Defined in: [src/lasa/roster.ts:34](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L34)

The live roster of a space, maintained from the presence track
with the `lasa-core.md` §7 receive rules applied. Obtain one from
[LasaClient.subscribeRoster](LasaClient.md#subscriberoster).

## Constructors

### Constructor

```ts
new PresenceRoster(): PresenceRoster;
```

#### Returns

`PresenceRoster`

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="onupdate"></a> `onUpdate?` | () => `void` | Called after any packet that changed the roster. | [src/lasa/roster.ts:40](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L40) |

## Methods

### apply()

```ts
apply(groupId, msg): void;
```

Defined in: [src/lasa/roster.ts:43](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L43)

Applies one presence packet under the §7 rules.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `groupId` | `bigint` |
| `msg` | \| [`PresenceKeyframe`](../interfaces/PresenceKeyframe.md) \| [`PresenceDelta`](../interfaces/PresenceDelta.md) |

#### Returns

`void`

***

### snapshot()

```ts
snapshot(): RosterEntry[];
```

Defined in: [src/lasa/roster.ts:92](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/roster.ts#L92)

The current roster, in unspecified order (entries are copies).

#### Returns

[`RosterEntry`](../interfaces/RosterEntry.md)[]
