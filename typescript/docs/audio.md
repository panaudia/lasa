# Audio

The client moves audio along two paths. Capture takes an `AudioNode`
on the page, encodes it to Opus in the transport worker, and sends it
as mono-object datagrams stamped with the entity's pose. Playout
subscribes a sink track the space renders for an entity, decodes it
in the worker, and plays it through an `AudioWorklet`. The page's
main thread never touches a sample on either path.

Both paths need cross-origin isolation (see the README) because
they hand audio between threads through `SharedArrayBuffer` rings.

## Capture

```ts
const ctx = new AudioContext({ sampleRate: 48000 });
const source = ctx.createMediaStreamSource(micStream);

const handle = await client.startCapture('alice-voice', {
  source,            // any AudioNode on a 48 kHz context
  channelIndex: 0,   // which channel of the source, default 0
  bitrate: 64000,    // Opus target, bits per second, default 64000
  pose: { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 },
});

handle.setPose(nextPose);   // at any rate, from any callback
await handle.stop();
```

The source's `AudioContext` MUST run at 48 kHz, because the wire
format is Opus at 48 kHz in 5 ms frames (`lasa-core.md` §5) and the
encoder is configured at the context's rate. `startCapture` refuses
any other rate with a `SAMPLE_RATE` error rather than shipping
non-conformant audio, so construct the context explicitly; a default
context follows the device and is often 44.1 kHz. The frame duration
is not an option. `bitrate` is the Opus target in bits per second,
an encoder setting that never touches the wire format: voice is
comfortable at 32 to 48 kb/s, a music or DAW source wants 96 to 128.

Each entity captures one mono channel. A stereo source becomes two
entities sharing one `source`, with `channelIndex` 0 and 1. A DAW
feeding a multi-channel node becomes one entity per channel.

`setPose` writes the freshest pose into a shared cell without
locking, and the next framed packet carries it. Call it from sensor
callbacks, `requestAnimationFrame`, or XR frame loops without
throttling. The pose is in LASA coordinates: see
[Coordinate systems](coordinates.md) for the converters.

The default frame is 5 ms, the LASA cadence. Longer frames lower the
packet rate at the cost of latency.

`client.getCaptureOverflows(entityId)` counts quanta the audio
thread dropped because the worker fell behind. A healthy pipeline
reports zero. Each overflow is a 128-sample gap on the uplink.

### Choosing a microphone

Bluetooth headsets switch to a low-quality, mono profile as soon as
any page opens their microphone, and some devices hold that state
afterwards. `selectBestMicrophone` ranks the available inputs by
label without opening any of them, preferring USB, then built-in,
then unknown, then Bluetooth.

```ts
const choice = await selectBestMicrophone();
if (choice.permissionPending) {
  // Labels are unreadable until permission is granted. Open the
  // production stream on the default device, then select again.
}
const mic = await navigator.mediaDevices.getUserMedia({
  audio: choice.deviceId ? { deviceId: { exact: choice.deviceId } } : true,
});
```

`micPermissionGranted` answers the permission question through the
Permissions API where it exists, and through the presence of device
labels where it does not.

## Playout

```ts
const player = await client.playSink('alice-voice', 'binaural', {
  qualityLevel: 0,
  latencyHint: 'interactive',
});
player.setVolume(0.8);
await client.stopSink('alice-voice');
```

The sink is the space's rendering for the entity: what the entity
hears from where it stands. Three formats exist, and a space MUST
offer at least the first two:

- `binaural` is a two-channel headphone render.
- `ambi2` is second-order ambisonics, nine channels.
- `ambi3` is third-order ambisonics, sixteen channels, offered where
  the space renders at that order or above.

`playSink` creates an `AudioContext` at 48 kHz. On browsers that
require a user gesture before audio plays, call it from a gesture
handler. The player calls `resume()` on the context for you.

### Quality and redundancy on the sink

`qualityLevel` and `redundancy` default from the entity's definition
(`LasaEntity.quality` and `LasaEntity.redundancy`) and MAY be
overridden per player. The sink direction is client-local: the
option changes this player's buffer, and nothing on the wire.

`qualityLevel` maps to the playout buffer's floor, robustness and
capacity. 0 holds the buffer at its lowest latency and expects a
clean network. 2 provisions for broadcast-grade tolerance.

`redundancy` above 0 turns on the reassembler in front of the
decoder, which repairs a lost frame from a later packet's repeat,
and raises the buffer floor so the repeat arrives in time.

### Watching the buffer

`player.getJitterStats()` returns the latest snapshot from the
worklet: fill level, target, underruns, overruns, trims and the
adaptive state. `player.getEvents()` returns the discontinuities
since `clearEvents()`, each timestamped at receipt on the main
thread.

`client.getSinkIngress(entityId)` reports the network side from the
worker: packets received, gap events, lost frames, reorders and
decode errors. A click with a gap event is the network's. A click
with an underrun is the buffer's.

`player.getProcessorError()` is null unless the worklet processor
has thrown. When it is set, the player is rendering silence and
should be disposed.

`player.getTapB()` reports the stereo-ness of the rendered output,
useful when a headset has collapsed to mono.
`probeOutputDeviceSampleRate` reports the output device's native
rate, which the 48 kHz context hides.

## The raw packet API

Underneath `startCapture` and `playSink` sit the packet primitives,
for applications that bring their own encoder or decoder.

```ts
const publisher = await client.entity('alice-voice');
await publisher.publish(seq++, { pose, audio: opusBytes });

await client.subscribeSink('alice-voice', 'binaural', (seq, packet) => {
  decoder.decode(new EncodedAudioChunk({ type: 'key', timestamp, data: packet.audio }));
});
```

`entity()` resolves once the server has subscribed the entity's
source track, normally within the first round trips. `publish`
sends one mono-object packet with `seq` as its datagram group id.
The sequence MUST be gapless and increasing.

A `MonoObjectPacket` carries any of a `pose`, an `audio` payload, and
a `redundancy` record repeating the audio from `offset` packets
back. A `SinkPacket` carries `audio`, an optional `redundancy`, and
an informational `timestampMicros`. Packets with unknown flags or a
malformed body are discarded before the handler sees them.
`MalformedError` and `UnknownFlagsError` are the exceptions the
parsers throw, exported for applications that parse for themselves.

`CapturePipeline` and `SinkPlayer` are also exported directly, for
applications composing the audio graph without a `LasaClient`.
