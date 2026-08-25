[@panaudia/lasa-client](../index.md) / CaptureOptions

# Interface: CaptureOptions

Defined in: [src/lasa/client.ts:56](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L56)

Options for [LasaClient.startCapture](../classes/LasaClient.md#startcapture).

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="bitrate"></a> `bitrate?` | `number` | Opus target bitrate in bits per second (default 64000, i.e. 64 kb/s). An encoder setting, not a wire-format one: voice is comfortable at 32–48 kb/s, a music or DAW source wants 96–128. Unrelated to the sample rate, which is always 48 kHz. | [src/lasa/client.ts:67](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L67) |
| <a id="channelindex"></a> `channelIndex?` | `number` | Which channel of the source (default 0) — stereo capture = two entities, indices 0/1. | [src/lasa/client.ts:60](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L60) |
| <a id="pose"></a> `pose?` | [`Pose`](Pose.md) | Initial pose (default: home). Update live via the handle. | [src/lasa/client.ts:69](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L69) |
| <a id="source"></a> `source` | `AudioNode` | The audio source feeding this entity (mic node, oscillator, DAW channel…). | [src/lasa/client.ts:58](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L58) |
