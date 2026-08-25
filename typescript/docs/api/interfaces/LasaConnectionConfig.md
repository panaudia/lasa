[@panaudia/lasa-client](../index.md) / LasaConnectionConfig

# Interface: LasaConnectionConfig

Defined in: [src/lasa/setup.ts:63](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L63)

The Connection Config (schemata/connection.schema.json).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="clientid"></a> `clientId` | `string` | - | [src/lasa/setup.ts:64](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L64) |
| <a id="entities"></a> `entities?` | [`LasaEntity`](LasaEntity.md)[] | Ad-hoc entity definitions (unticketed, or presenter/admin tickets). | [src/lasa/setup.ts:68](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L68) |
| <a id="setups"></a> `setups?` | [`LasaSetup`](LasaSetup.md)[] | Instantiations of ticket-defined entities. | [src/lasa/setup.ts:70](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L70) |
| <a id="ticket"></a> `ticket?` | `string` | Signed JWT ticket; omit only against a space allowing unticketed. | [src/lasa/setup.ts:66](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/setup.ts#L66) |
