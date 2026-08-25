[@panaudia/lasa-client](../index.md) / LasaRejectionError

# Class: LasaRejectionError

Defined in: [src/lasa/errors.ts:65](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L65)

A LASA session rejection: the server deliberately closed the session
with a §4.6 code. `code` is normative; `reason` is advisory.

## Extends

- `Error`

## Constructors

### Constructor

```ts
new LasaRejectionError(code, reason): LasaRejectionError;
```

Defined in: [src/lasa/errors.ts:66](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L66)

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
| <a id="code"></a> `code` | `readonly` | `number` | [src/lasa/errors.ts:67](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L67) |
| <a id="reason"></a> `reason` | `readonly` | `string` | [src/lasa/errors.ts:68](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L68) |
