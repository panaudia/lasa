[@panaudia/lasa-client](../index.md) / StereoMeterReport

# Interface: StereoMeterReport

Defined in: [src/audio/stereo-meter-core.ts:24](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L24)

One analysis window's result. Linear amplitudes (not dB); convert UI-side.

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="correlation"></a> `correlation` | `number` | Normalized L/R correlation at lag 0, −1…+1. 0 when either channel is ~silent. | [src/audio/stereo-meter-core.ts:34](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L34) |
| <a id="frames"></a> `frames` | `number` | Frames (sample pairs) in the window. 0 ⇒ all other fields are 0. | [src/audio/stereo-meter-core.ts:26](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L26) |
| <a id="midrms"></a> `midRms` | `number` | RMS of (L+R)/2 — the mono ("mid") content. | [src/audio/stereo-meter-core.ts:30](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L30) |
| <a id="rmsl"></a> `rmsL` | `number` | - | [src/audio/stereo-meter-core.ts:27](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L27) |
| <a id="rmsr"></a> `rmsR` | `number` | - | [src/audio/stereo-meter-core.ts:28](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L28) |
| <a id="siderms"></a> `sideRms` | `number` | RMS of (L−R)/2 — the stereo difference ("side"). ~0 ⇒ mono collapse. | [src/audio/stereo-meter-core.ts:32](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/stereo-meter-core.ts#L32) |
