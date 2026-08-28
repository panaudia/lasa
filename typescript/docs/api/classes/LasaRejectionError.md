[@panaudia/lasa-client](../index.md) / LasaRejectionError

# Class: LasaRejectionError

Defined in: [src/lasa/errors.ts:71](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L71)

A LASA session rejection: the server deliberately closed the session
with a §4.6 code. `code` is normative; `reason` is advisory.

## Extends

- `Error`

## Constructors

### Constructor

```ts
new LasaRejectionError(code, reason): LasaRejectionError;
```

Defined in: [src/lasa/errors.ts:72](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L72)

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `code` | `number` |
| `reason` | `string` |

#### Returns

`LasaRejectionError`

#### Overrides

```ts
Error.constructor
```

## Properties

| Property | Modifier | Type | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="code"></a> `code` | `readonly` | `number` | [src/lasa/errors.ts:73](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L73) |
| <a id="reason"></a> `reason` | `readonly` | `string` | [src/lasa/errors.ts:74](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L74) |
