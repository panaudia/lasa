[@panaudia/lasa-client](../index.md) / selectBestMicrophone

# Function: selectBestMicrophone()

```ts
function selectBestMicrophone(debug?): Promise<MicrophoneSelectionResult>;
```

Defined in: [src/audio/microphone-selection.ts:236](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L236)

Select the best non-Bluetooth microphone WITHOUT opening any device.

Flow:
1. If permission is not yet granted: return `permissionPending` — labels
   are unreadable and this module never opens a device to read them (see
   module doc). The caller opens its production stream on the default and
   re-runs selection afterwards.
2. Enumerate all audio input devices and classify by label.
3. Return the highest-ranked non-Bluetooth device, or fall back to default.

## Parameters

| Parameter | Type | Default value | Description |
| ------ | ------ | ------ | ------ |
| `debug` | `boolean` | `false` | Enable console logging of the selection process. |

## Returns

`Promise`\<[`MicrophoneSelectionResult`](../interfaces/MicrophoneSelectionResult.md)\>
