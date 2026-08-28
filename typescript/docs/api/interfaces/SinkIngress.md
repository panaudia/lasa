[@panaudia/lasa-client](../index.md) / SinkIngress

# Interface: SinkIngress

Defined in: [src/lasa/client.ts:88](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L88)

Ingress health of a decoded sink track (see worker protocol).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="decodeerrors"></a> `decodeErrors` | `number` | Frames the decoder rejected. | [src/lasa/client.ts:98](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L98) |
| <a id="gapevents"></a> `gapEvents` | `number` | Runs of one or more missing sequence numbers. | [src/lasa/client.ts:92](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L92) |
| <a id="lostframes"></a> `lostFrames` | `number` | Frames never received (after any redundancy repair). | [src/lasa/client.ts:94](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L94) |
| <a id="received"></a> `received` | `number` | Sink packets received. | [src/lasa/client.ts:90](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L90) |
| <a id="reordered"></a> `reordered` | `number` | Packets that arrived out of sequence order. | [src/lasa/client.ts:96](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L96) |
