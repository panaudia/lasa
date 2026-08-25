[@panaudia/lasa-client](../index.md) / MicrophoneSelectionResult

# Interface: MicrophoneSelectionResult

Defined in: [src/audio/microphone-selection.ts:125](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L125)

## Properties

| Property | Type | Description | Defined in |
| ------ | ------ | ------ | ------ |
| <a id="alldevices"></a> `allDevices` | [`ClassifiedMicrophone`](ClassifiedMicrophone.md)[] | All devices that were evaluated. | [src/audio/microphone-selection.ts:133](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L133) |
| <a id="deviceid"></a> `deviceId` | `string` \| `undefined` | The selected device ID (or undefined to use system default). | [src/audio/microphone-selection.ts:127](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L127) |
| <a id="label"></a> `label` | `string` | The label of the selected device. | [src/audio/microphone-selection.ts:129](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L129) |
| <a id="permissionpending"></a> `permissionPending` | `boolean` | True when mic permission has not been granted yet: labels are unreadable and no device was (or ever will be, by this module) opened to read them. The caller should open its production stream on the default device, then re-run selection — permission now exists — and switch away if the default proved Bluetooth. | [src/audio/microphone-selection.ts:143](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L143) |
| <a id="switchedfrombluetooth"></a> `switchedFromBluetooth` | `boolean` | Whether the default mic was Bluetooth and we switched away from it. | [src/audio/microphone-selection.ts:135](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L135) |
| <a id="type"></a> `type` | [`MicrophoneType`](../type-aliases/MicrophoneType.md) | Classification of the selected device. | [src/audio/microphone-selection.ts:131](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L131) |
