# test-page

Browser test instrument for `@panaudia/lasa-client`: every subsystem
of the SDK on one page, against a running LASA space. For a starting
point to copy, see `../minimal` instead; this page is the thing you
open when something needs looking at.

Every panel exercises a subsystem:

- **Connection** — the Connection Config at CLIENT_SETUP (ticket or
  unticketed). Entities are declared *before* connecting: the entity
  set is fixed per connection by the protocol.
- **Entities** — one capture pipeline per entity (test tone or
  microphone), one binaural sink playout per entity. Both run the
  3-thread architecture: no audio touches the main thread.
- **Space map** — drag your entities around; presence drives everyone
  else. Dragging a capturing entity updates its pose through the
  shared-memory pose cell (the freshest-pose path); non-capturing
  entities send pose-only packets. The yaw slider steers the last
  entity you dragged — with a sink playing, this is the head-rotation
  latency made audible.
- **State** — the live synced store, plus set/clear of arbitrary keys
  (subject to the space's rules; unticketed connections can write
  their own entity's `attrs.*` and anything outside `lasa.`).
- **Diagnostics** — per-playing-sink jitter-buffer stats (fill, L/H
  allowances, underruns), the Tap B stereo meter of the rendered
  output, and the device's reported audio latencies.

## Run

Needs a running space. From `panaudia-lasa/server` (sibling checkout
of this repository):

```bash
PANAUDIA_ALLOW_UNTICKETED=true PANAUDIA_PORT=4443 PANAUDIA_REVERB=none go run .
```

Then here:

```bash
npm install
npm run dev
```

Open `http://127.0.0.1:5300/` — or, with the wildcard cert in place
(see below), `https://dev.panaudia.com:5300/`. The dev server sends
COOP/COEP headers (the SDK's capture/playout require cross-origin
isolation) — the header shows whether isolation is active.

## Certificates

Two options, matching the SDK test rig:

- **Dev cert hash** (quickest): the server logs
  `serverCertificateHashes (sha-256, base64): …` at boot when using
  its self-signed dev cert. Paste that into the *Cert hash* field.
  Chromium-family browsers accept it via
  `WebTransport serverCertificateHashes`; for engines that do not
  support cert hashes, use the second option.
- **Trusted certificate**: point the server at a real or locally
  trusted cert (`PANAUDIA_CERT`/`PANAUDIA_KEY`), e.g. the
  `*.panaudia.com` wildcard with `127.0.0.1 dev.panaudia.com` in
  `/etc/hosts`, and connect to
  `https://dev.panaudia.com:4443/lasa` with the hash field blank.
  When those keys exist at `../../../keys/server.{crt,key}` (or
  `PANAUDIA_DEV_KEYS_DIR` points at them), the dev server also
  serves the **page** over HTTPS — browse
  `https://dev.panaudia.com:5300/`. That matters because a
  non-localhost plain-HTTP origin is not a secure context: no
  WebTransport, no cross-origin isolation. (`127.0.0.1` is exempt,
  which is why the plain-HTTP route works there.)

## Notes

- The SDK is consumed **straight from source** via a Vite alias
  (`vite.config.ts`), so the page always exercises the tree it sits
  in, with no build step. `../minimal` takes the normal dependency
  route instead.
- Microphone capture uses the default input device with echo
  cancellation, noise suppression, and auto-gain all off (the SDK's
  music/spatial defaults). Bluetooth microphones will drag the whole
  device into the low-quality HFP profile — use a wired mic.
- Two browser tabs (different client ids) make a two-person space:
  capture in one, listen in the other, drag poses in either.
