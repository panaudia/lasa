[@panaudia/lasa-client](../index.md) / LasaClientOptions

# Interface: LasaClientOptions

Defined in: [src/lasa/client.ts:95](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L95)

Options for [LasaClient.connect](../classes/LasaClient.md#connect). See `docs/connecting.md`.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="clientid"></a> `clientId` | `string` | This client's public id within the space. With a ticket it MUST match the `client_id` claim. | [src/lasa/client.ts:101](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L101) |
| <a id="debug"></a> `debug?` | `boolean` | Console logging from the client, worker and audio graph. | [src/lasa/client.ts:111](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L111) |
| <a id="entities"></a> `entities?` | [`LasaEntity`](LasaEntity.md)[] | Ad-hoc entity definitions (unticketed spaces, or presenter/admin tickets). | [src/lasa/client.ts:105](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L105) |
| <a id="onclosed"></a> `onClosed?` | (`info`) => `void` | The session closed after a completed handshake — a moderation sever, a supersession by a newer connection exercising this client id (lasa-core.md §4.4), or a network/server failure. closeCode carries a LASA §4.6 code when the close was a deliberate LASA action; a rejected handshake surfaces as a LasaRejectionError from connect() instead, never here. | [src/lasa/client.ts:120](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L120) |
| <a id="servercertificatehashbase64"></a> `serverCertificateHashBase64?` | `string` | Dev-cert pinning (base64 SHA-256 from the server boot log). | [src/lasa/client.ts:109](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L109) |
| <a id="setups"></a> `setups?` | [`LasaSetup`](LasaSetup.md)[] | Instantiations of ticket-defined entities, by id. | [src/lasa/client.ts:107](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L107) |
| <a id="spaceid"></a> `spaceId` | `string` | The space to join. In a ticketed space this MUST equal the ticket's `aud` claim. | [src/lasa/client.ts:99](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L99) |
| <a id="ticket"></a> `ticket?` | `string` | Signed JWT ticket. Required by a ticketed space, refused by an unticketed one. | [src/lasa/client.ts:103](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L103) |
| <a id="url"></a> `url` | `string` | WebTransport endpoint, e.g. https://host:4443/lasa | [src/lasa/client.ts:97](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L97) |
