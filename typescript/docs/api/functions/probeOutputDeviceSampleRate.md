[@panaudia/lasa-client](../index.md) / probeOutputDeviceSampleRate

# Function: probeOutputDeviceSampleRate()

```ts
function probeOutputDeviceSampleRate(): Promise<number | null>;
```

Defined in: [src/audio/player.ts:423](https://github.com/panaudia/lasa/blob/main/typescript/src/audio/player.ts#L423)

Probe the OUTPUT device's native sample rate (ported from the old
SDK's stereo-diagnostics).

The playout AudioContext requests 48 kHz explicitly, so its
`sampleRate` never reflects the device. A throwaway default-rate
context inherits the current output device's rate — a Bluetooth
headset stuck in HFP typically reports 16–24 kHz instead of
44.1/48 k. This is the only in-browser signal for the "everything
says stereo but it SOUNDS mono" state: HFP collapse happens in the
OS below every Web Audio observable. Heuristic — a low rate strongly
suggests HFP; a normal rate does not fully rule it out.

Returns null where AudioContext is unavailable or construction fails.

## Returns

`Promise`\<`number` \| `null`\>
