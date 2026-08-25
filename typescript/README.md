# @panaudia/lasa-client

The TypeScript client for LASA, live spatial audio over Media over
QUIC. It runs in the browser and speaks to a LASA space over
WebTransport: it publishes object audio with pose, plays rendered
sinks, follows the presence feed, and synchronises the space's
distributed state store.

The protocol lives one directory up, in `lasa-core.md` and
`lasa-base-profile.md`. This package is the browser half of it. The
server half is [Panaudia LASA](https://github.com/panaudia/panaudia-lasa).

## Requirements

The client uses four browser platform features, and every one is
mandatory:

- WebTransport, for the QUIC connection.
- WebCodecs, for Opus encode and decode (`AudioEncoder` and
  `AudioDecoder`).
- AudioWorklet, for capture and playout on the audio thread.
- SharedArrayBuffer, for the lock-free rings between the audio
  thread, the transport worker and the page.

SharedArrayBuffer needs cross-origin isolation. Your page MUST be
served with these two headers, or capture and playout refuse to
start:

```
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
```

Chromium is the engine the interop suite runs against. Other engines
work as their WebTransport and WebCodecs support allows.

The package is ESM only and targets ES2022. There are no runtime
dependencies.

## Install

```
npm install @panaudia/lasa-client
```

The transport worker is inlined into the bundle as a Blob, so there
is no worker file to copy and nothing for your bundler to resolve.

## A first session

```ts
import { LasaClient } from '@panaudia/lasa-client';

const client = await LasaClient.connect({
  url: 'https://lasa.example.com:4443/lasa',
  spaceId: 'main',
  clientId: 'alice',
  entities: [{ id: 'alice-voice', name: 'Alice' }],
});

// Capture: the microphone into the entity, five-millisecond Opus
// frames, each stamped with the freshest pose.
const mic = await navigator.mediaDevices.getUserMedia({ audio: true });
const ctx = new AudioContext({ sampleRate: 48000 });
const source = ctx.createMediaStreamSource(mic);
const capture = await client.startCapture('alice-voice', { source });
capture.setPose({ x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0 });

// Playout: the space renders a binaural mix for this entity, the
// client decodes it in the worker and plays it through a worklet.
const player = await client.playSink('alice-voice', 'binaural');
player.setVolume(0.8);

// Presence: who else is here, and where.
const roster = await client.subscribeRoster(() => {
  for (const entry of roster.snapshot()) {
    console.log(entry.id, entry.pose, entry.loudness);
  }
});

// Later.
await client.close();
```

`playSink` builds an `AudioContext`, so on browsers that gate audio
behind a user gesture, call it from a click or key handler.

## Guides

- [Connecting](docs/connecting.md) covers tickets, entities and
  setups, certificate pinning for development servers, rejection
  codes, and the session lifecycle.
- [Audio](docs/audio.md) covers capture, playout, sink formats,
  quality and redundancy, microphone selection, and the raw packet
  API underneath.
- [Presence](docs/presence.md) covers the roster and the receive
  rules it applies.
- [State](docs/state.md) covers the store, synchronisation, writes,
  reconnects and cursors.
- [Base profile](docs/profile.md) covers the typed helpers over the
  `lasa.` vocabulary: personal mutes and solos, moderation, channels,
  rendering parameters, and the entity view.
- [Coordinate systems](docs/coordinates.md) covers the LASA pose and
  the converters for Three.js, Babylon, A-Frame, PlayCanvas, Unity,
  Unreal and Pixi.

The [API reference](docs/api/index.md) is generated from the source
with TypeDoc, as markdown, and committed. `npm run docs:api`
regenerates it after a change to the exported surface.

## Development

```
npm install
npm run typecheck      # tsc
npm test               # vitest unit suite
npm run test:interop   # Playwright, against a server built from ../../panaudia-lasa
npm run build          # dist/index.js + declarations
```

The interop suite builds and launches the Go server itself, one
instance in unticketed dev mode and one ticketed, and drives a real
Chromium against both. `npm run dev` serves the same interop page
with the isolation headers set.

`PROVENANCE.md` records what this package inherits from the earlier
Panaudia SDK and what was pruned on the way.

## Licence

MIT. See `LICENSE`.
