[@panaudia/lasa-client](../index.md) / LasaClientOptions

# Interface: LasaClientOptions

Defined in: [src/lasa/client.ts:102](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L102)

Options for [LasaClient.connect](../classes/LasaClient.md#connect). See `docs/connecting.md`.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="clientid"></a> `clientId` | `string` | This client's public id within the space. With a ticket it MUST match the `client_id` claim. | [src/lasa/client.ts:108](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L108) |
| <a id="debug"></a> `debug?` | `boolean` | Console logging from the client, worker and audio graph. | [src/lasa/client.ts:118](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L118) |
| <a id="entities"></a> `entities?` | [`LasaEntity`](LasaEntity.md)[] | Ad-hoc entity definitions (unticketed spaces, or presenter/admin tickets). | [src/lasa/client.ts:112](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L112) |
| <a id="onclosed"></a> `onClosed?` | (`info`) => `void` | The session closed after a completed handshake — a moderation sever, a supersession by a newer connection exercising this client id (lasa-core.md §4.4), or a network/server failure. closeCode carries a LASA §4.6 code when the close was a deliberate LASA action; a rejected handshake surfaces as a LasaRejectionError from connect() instead, never here. | [src/lasa/client.ts:127](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L127) |
| <a id="servercertificatehashbase64"></a> `serverCertificateHashBase64?` | `string` | Dev-cert pinning (base64 SHA-256 from the server boot log). | [src/lasa/client.ts:116](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L116) |
| <a id="setups"></a> `setups?` | [`LasaSetup`](LasaSetup.md)[] | Instantiations of ticket-defined entities, by id. | [src/lasa/client.ts:114](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L114) |
| <a id="spaceid"></a> `spaceId` | `string` | The space to join. In a ticketed space this MUST equal the ticket's `aud` claim. | [src/lasa/client.ts:106](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L106) |
| <a id="ticket"></a> `ticket?` | `string` | Signed JWT ticket. Required by a ticketed space, refused by an unticketed one. | [src/lasa/client.ts:110](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L110) |
| <a id="url"></a> `url` | `string` | WebTransport endpoint, e.g. https://host:4443/lasa | [src/lasa/client.ts:104](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L104) |
