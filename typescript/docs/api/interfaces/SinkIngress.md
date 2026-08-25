[@panaudia/lasa-client](../index.md) / SinkIngress

# Interface: SinkIngress

Defined in: [src/lasa/client.ts:81](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L81)

Ingress health of a decoded sink track (see worker protocol).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="decodeerrors"></a> `decodeErrors` | `number` | Frames the decoder rejected. | [src/lasa/client.ts:91](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L91) |
| <a id="gapevents"></a> `gapEvents` | `number` | Runs of one or more missing sequence numbers. | [src/lasa/client.ts:85](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L85) |
| <a id="lostframes"></a> `lostFrames` | `number` | Frames never received (after any redundancy repair). | [src/lasa/client.ts:87](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L87) |
| <a id="received"></a> `received` | `number` | Sink packets received. | [src/lasa/client.ts:83](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L83) |
| <a id="reordered"></a> `reordered` | `number` | Packets that arrived out of sequence order. | [src/lasa/client.ts:89](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L89) |
