# minimal

The smallest useful LASA client, and the starting point to copy: one
page that joins a space, sends the microphone as one entity, plays
the space's binaural render of everyone else, and lists who is there
with names and positions. About a hundred lines in `src/main.ts`,
all of them the public package surface.

## Run

Needs a running space. From `panaudia-lasa/server` (a sibling
checkout of this repository):

```bash
PANAUDIA_ALLOW_UNTICKETED=true PANAUDIA_PORT=4443 PANAUDIA_REVERB=none go run .
```

The server logs `serverCertificateHashes (sha-256, base64): …` at boot
when it is using its self-signed development certificate. Then here:

```bash
npm install
npm run dev
```

Open `http://127.0.0.1:5301/`, paste the certificate hash, pick a
client id, and join. A second tab with a different client id gives
you someone to hear. Wired headphones; a Bluetooth microphone drags
the device into the low-quality HFP profile.

## What it shows

- `LasaClient.connect` with one ad-hoc entity, no ticket.
- `startCapture` from a 48 kHz `AudioContext`, with the browser's
  voice processing switched off.
- `playSink` of the `binaural` format.
- `subscribeRoster` for presence, and `Store` + `baseView` for the
  base-profile names.
- `close()` tearing all of it down.

## How it consumes the package

`package.json` depends on `"@panaudia/lasa-client": "file:../.."`, the
built package two directories up, which is what a published
dependency would resolve to. Build it first (`npm run build` in
`../..`) or the import has nothing to find. Once the package is on
npm, replace the `file:` reference with a version.

`vite.config.ts` is two lines of substance: the COOP and COEP headers
that make the page cross-origin isolated, which capture and playout
require. Any host you deploy to needs to send the same two headers.
