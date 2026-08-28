[@panaudia/lasa-client](../index.md) / CaptureOptions

# Interface: CaptureOptions

Defined in: [src/lasa/client.ts:54](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L54)

Options for [LasaClient.startCapture](../classes/LasaClient.md#startcapture).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="bitrate"></a> `bitrate?` | `number` | Opus target bitrate in bits per second (default 64000, i.e. 64 kb/s). An encoder setting, not a wire-format one: voice is comfortable at 32–48 kb/s, a music or DAW source wants 96–128. Unrelated to the sample rate, which is always 48 kHz. | [src/lasa/client.ts:65](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L65) |
| <a id="channelindex"></a> `channelIndex?` | `number` | Which channel of the source (default 0) — stereo capture = two entities, indices 0/1. | [src/lasa/client.ts:58](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L58) |
| <a id="pose"></a> `pose?` | [`Pose`](Pose.md) | Initial pose (default: home). Update live via the handle. | [src/lasa/client.ts:67](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L67) |
| <a id="redundancy"></a> `redundancy?` | `number` | Uplink redundancy offset for this entity (0–7). Defaults to the entity's declaration (`LasaEntity.redundancy`), which is also the ceiling: the server provisioned its ingest buffer from it, and an offset beyond it is a sender violation (lasa-core.md §5.1). Lower it to trade robustness for uplink bandwidth. For a ticket-defined entity the client cannot see the declaration and accepts 0–7. | [src/lasa/client.ts:76](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L76) |
| <a id="source"></a> `source` | `AudioNode` | The audio source feeding this entity (mic node, oscillator, DAW channel…). | [src/lasa/client.ts:56](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L56) |
