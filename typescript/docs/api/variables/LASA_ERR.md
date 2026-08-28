[@panaudia/lasa-client](../index.md) / LASA\_ERR

# Variable: LASA\_ERR

```ts
const LASA_ERR: object;
```

Defined in: [src/lasa/errors.ts:10](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L10)

Session error codes, lasa-core.md §4.6 / §9.

## Type Declaration

| Name | Type | Default value | Description | Defined in |
| ------ | ------ | ------ | ------ | ------ |
| <a id="property-ad_hoc_not_permitted"></a> `AD_HOC_NOT_PERMITTED` | `1726981` | `0x1a5a05` | Ad-hoc entities not permitted for this ticket. | [src/lasa/errors.ts:22](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L22) |
| <a id="property-at_capacity"></a> `AT_CAPACITY` | `1726983` | `0x1a5a07` | Space at capacity. | [src/lasa/errors.ts:26](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L26) |
| <a id="property-blocked"></a> `BLOCKED` | `1726982` | `0x1a5a06` | Blocked (reason string SHOULD carry the block expiry timestamp). | [src/lasa/errors.ts:24](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L24) |
| <a id="property-client_id_mismatch"></a> `CLIENT_ID_MISMATCH` | `1726978` | `0x1a5a02` | client_id does not match the ticket claim. | [src/lasa/errors.ts:16](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L16) |
| <a id="property-entity_collision"></a> `ENTITY_COLLISION` | `1726980` | `0x1a5a04` | Entity id collides with another client's live entity (ticketed). | [src/lasa/errors.ts:20](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L20) |
| <a id="property-id_collision_retired"></a> `ID_COLLISION_RETIRED` | `1726979` | `0x1a5a03` | RETIRED 2026-08-13 (unticketed collisions now supersede); never reused. | [src/lasa/errors.ts:18](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L18) |
| <a id="property-invalid_ticket"></a> `INVALID_TICKET` | `1726977` | `0x1a5a01` | Invalid ticket (signature, expired, not yet valid, wrong audience) — or no ticket presented to a ticketed space. | [src/lasa/errors.ts:14](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L14) |
| <a id="property-malformed_config"></a> `MALFORMED_CONFIG` | `1726976` | `0x1a5a00` | Malformed / schema-invalid Connection Config. | [src/lasa/errors.ts:12](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L12) |
| <a id="property-mode_mismatch"></a> `MODE_MISMATCH` | `1726984` | `0x1a5a08` | Ticket presented to an unticketed space (mode mismatch). | [src/lasa/errors.ts:28](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L28) |
| <a id="property-shutting_down"></a> `SHUTTING_DOWN` | `1726985` | `0x1a5a09` | The space is shutting down deliberately; reconnecting later is reasonable. | [src/lasa/errors.ts:30](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L30) |
| <a id="property-superseded"></a> `SUPERSEDED` | `1726986` | `0x1a5a0a` | Superseded by a newer connection exercising the same client id (§4.4). | [src/lasa/errors.ts:32](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/errors.ts#L32) |
