[@panaudia/lasa-client](../index.md) / micPermissionGranted

# Function: micPermissionGranted()

```ts
function micPermissionGranted(): Promise<boolean>;
```

Defined in: [src/audio/microphone-selection.ts:198](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/microphone-selection.ts#L198)

Whether mic permission is ALREADY granted. When it is, enumerateDevices()
returns full labels without opening any microphone — critical for Bluetooth
headsets, where merely opening the mic (even our brief label-population
probe) flips the headset into HFP and collapses its output to mono, a state
macOS/the device can then hold on to. Where the Permissions API is
unavailable or does not know the `microphone` name (Firefox throws a
TypeError), falls back to enumerateDevices(): a non-empty input label is
only ever readable once permission has been granted, and enumeration
opens nothing.

## Returns

`Promise`\<`boolean`\>
