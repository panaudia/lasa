[@panaudia/lasa-client](../index.md) / MalformedError

# Class: MalformedError

Defined in: [src/lasa/wire.ts:14](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/wire.ts#L14)

A packet or message that violates its format. Receivers discard it.

## Extends

- `Error`

## Constructors

### Constructor

```ts
new MalformedError(message?): MalformedError;
```

Defined in: node\_modules/typescript/lib/lib.es5.d.ts:1082

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `message?` | `string` |

#### Returns

`MalformedError`

#### Inherited from

```ts
Error.constructor
```

### Constructor

```ts
new MalformedError(message?, options?): MalformedError;
```

Defined in: node\_modules/typescript/lib/lib.es5.d.ts:1082

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `message?` | `string` |
| `options?` | `ErrorOptions` |

#### Returns

`MalformedError`

#### Inherited from

```ts
Error.constructor
```
