# Browser audio playback options for MoQ streaming

*Research notes, 2026-08-04. Question: for low-latency audio received over MoQ in a browser — with custom de-jitter and drift correction applied before handing samples to the OS — is AudioWorklet + SharedArrayBuffer (SAB) really the only viable path, and what are the alternatives?*

## Conclusion

**Yes — SAB + AudioWorklet is the only path that supports custom de-jitter and drift correction off the main thread with deterministic timing.** Nothing new on the platform (WASM included) avoids the cross-origin-isolation requirement, and nothing coming will. However, the *deployment* story for cross-origin isolation has improved materially: `COEP: credentialless` and, in Chrome 137+, `Document-Isolation-Policy` (DIP) remove most of the historical pain — including, crucially, the embedded-iframe problem.

Practical finding from this project: postMessage delivery into a worklet is too irregular to sustain low-latency streaming without adding ~100 ms of extra buffering, which is unacceptable here. That rules out every non-SAB option in practice.

## Corrections to common assumptions

### "WASM shared memory" is not an escape hatch

WASM threads' shared linear memory **is a `SharedArrayBuffer`** under the hood, so it is gated on `crossOriginIsolated` exactly like JS SAB. The thing that is "not available yet" is the [shared-everything-threads proposal](https://github.com/WebAssembly/shared-everything-threads) (sharing WasmGC objects, native thread spawning) — still a draft, with a [Chrome implementation in progress](https://chromestatus.com/feature/5163209685467136) — and it will carry the same isolation requirement. Waiting for WASM developments will not remove the header requirement.

### The requirement is COOP/COEP (cross-origin isolation), not CORS

What SAB needs is `crossOriginIsolated === true`, achieved via `Cross-Origin-Opener-Policy` + `Cross-Origin-Embedder-Policy` response headers — or, in Chrome 137+, via the new `Document-Isolation-Policy` header (see below). As of mid-2026 there is no relaxation of this requirement in any browser.

## The options compared

### 1. AudioWorklet + SAB ring buffer ✅ (chosen)

Requires cross-origin isolation (COOP/COEP or DIP).

- **Pros:** the gold standard. Wait-free, zero-copy, sample-accurate. The worklet owns the buffer, so de-jitter and drift correction (occupancy-driven micro-resampling / varispeed) work exactly as designed. Canonical implementation: [ringbuf.js](https://github.com/padenot/ringbuf.js/) (SPSC, wait-free), design rationale in [Paul Adenot's write-up](https://blog.paul.cx/post/a-wait-free-spsc-ringbuffer-for-the-web/).
- **Cons:** isolation headers must be deployed on the serving origin (see deployment section for mitigations).

### 2. AudioWorklet fed by postMessage from a Worker ❌ (tested, rejected)

Transfer the `AudioWorkletNode.port` (MessagePort is transferable) to the network/decode worker; the worker posts decoded PCM directly into the worklet. Delivery happens on the render thread between quanta, so main-thread jank is irrelevant after setup.

- **Pros:** no isolation headers; the worklet still owns the queue, so drift-correction code is identical to option 1. Endorsed by the moq ecosystem — [kixelated's "To WASM, or not to WASM"](https://moq.dev/blog/to-wasm/) argues postMessage costs "a few milliseconds at most," negligible under a ~100 ms network jitter buffer.
- **Cons — disqualifying here:** per-message allocation/GC pressure, and *nondeterministic delivery timing*. Tested in this project: delivery is too irregular to sustain low latency without an extra ~100 ms buffer to absorb the jitter. No cheap backpressure signal either.

### 3. WebCodecs `AudioData` → `MediaStreamTrackGenerator` (in worker) → `<audio>` ❌

- **Pros:** fully off main thread; browser handles playout.
- **Cons:** the browser owns the playout buffer, so **custom de-jitter/drift correction is impossible** — disqualifying. Also Chrome-only: the spec was [unbundled to video-only](https://blog.mozilla.org/webrtc/unbundling-mediastreamtrackprocessor-and-videotrackgenerator/) (`VideoTrackGenerator`); Mozilla opposes the audio variant; Safari's `MediaStreamTrackProcessor` is video-only.

### 4. MSE in a dedicated worker ❌

Mux to fMP4/WebM, pass a `MediaSourceHandle` to a media element ([`MediaSource.canConstructInDedicatedWorker`](https://developer.mozilla.org/en-US/docs/Web/API/MediaSource/canConstructInDedicatedWorker_static); Chrome 108+, Safari 17.1+).

- **Pros:** no isolation needed, off main thread, coarse drift control via `playbackRate` nudging.
- **Cons:** container muxing overhead; buffering model built for streaming, not real-time — realistic latency floor is hundreds of ms; drift/jitter handling is coarse and partly opaque.

### 5. Main-thread `AudioBufferSourceNode` scheduling ❌

Any main-thread jank becomes an audible glitch. Ruled out by the core constraint.

### 6. WebRTC loopback (local `RTCPeerConnection` to borrow NetEQ) ❌

Exists as a hack; costs an Opus re-encode or insertable-streams contortions, and cedes all drift control to NetEQ. Not worth it.

## Deploying cross-origin isolation with least pain

### Baseline (all browsers today)

`Cross-Origin-Opener-Policy: same-origin` + `Cross-Origin-Embedder-Policy` on the document ([web.dev guide](https://web.dev/articles/coop-coep)).

- Prefer `COEP: credentialless` when loading cross-origin subresources you don't control (strips credentials instead of requiring CORP opt-in). Supported in Chrome and Firefox; **Safari support has lagged — verify**, and fall back to `require-corp` (with `Cross-Origin-Resource-Policy` headers on your own subresources) if Safari matters.
- Known costs: COOP `same-origin` severs `window.opener`, so OAuth-style popups need workarounds; and under COOP/COEP the player only works inside a third-party iframe if the *embedder* is isolated and delegates `allow="cross-origin-isolated"`.

### Progressive upgrade: `Document-Isolation-Policy` (Chrome 137+ desktop)

Additionally send `Document-Isolation-Policy: isolate-and-credentialless` ([Chrome announcement](https://developer.chrome.com/blog/document-isolation-policy), [WICG explainer](https://github.com/WICG/document-isolation-policy)).

- Per-document/per-frame; doesn't break popups; imposes nothing on embedders or embedded content.
- **Key finding:** per the WICG explainer, an embedded iframe with a DIP header becomes `crossOriginIsolated` *regardless of whether the embedding page deployed COI* — no permissions-policy delegation needed. This removes the worst deployment problem for embeddable SAB-based players. Quote: "An embedded iframe can deploy a Document-Isolation-Policy of `isolated-and-credentialless` or `isolated-and-require-corp` and gain access to COI-gated APIs … regardless of whether its embedder deployed COI or not."
- Sending both DIP and COOP/COEP is safe: Chrome takes the better path; other browsers ignore DIP. Chrome-only for now (Android planned; Firefox/Safari not yet).

### No control over response headers (static hosting)

The [coi-serviceworker](https://github.com/gzuidhof/coi-serviceworker) shim injects COOP/COEP via a service worker. Works at the cost of a one-time reload on first visit and the usual service-worker caveats. Fine for demos, questionable for production.

## Implementation notes for the worklet

- Never block in `process()` — read the ring buffer non-blockingly each quantum; let buffer occupancy drive skew/varispeed drift correction.
- `Atomics.waitAsync` is available on the producer side for wakeups without spinning (SAB-only, so fine here).
- `AudioContext.setSinkId()` covers output-device selection — nothing is lost by staying off media elements.
- Forward-looking: WebAudio has long-standing proposals for a built-in streaming/push node (e.g. [web-audio-api#1322](https://github.com/WebAudio/web-audio-api/issues/1322)) but nothing has shipped or is close; SAB + worklet remains the intended design for this workload.

## Sources

- [WICG Document-Isolation-Policy explainer](https://github.com/WICG/document-isolation-policy)
- [Chrome: Document Isolation Policy announcement](https://developer.chrome.com/blog/document-isolation-policy)
- [web.dev: Making your website cross-origin isolated using COOP and COEP](https://web.dev/articles/coop-coep)
- [moq.dev: To WASM, or not to WASM](https://moq.dev/blog/to-wasm/)
- [WebAssembly shared-everything-threads proposal](https://github.com/WebAssembly/shared-everything-threads) · [Chrome status](https://chromestatus.com/feature/5163209685467136)
- [ringbuf.js](https://github.com/padenot/ringbuf.js/) · [A wait-free SPSC ring buffer for the Web](https://blog.paul.cx/post/a-wait-free-spsc-ringbuffer-for-the-web/)
- [Chrome: Audio Worklet design pattern](https://developer.chrome.com/blog/audio-worklet-design-pattern/)
- [Mozilla: Unbundling MediaStreamTrackProcessor and VideoTrackGenerator](https://blog.mozilla.org/webrtc/unbundling-mediastreamtrackprocessor-and-videotrackgenerator/)
- [MDN: MediaSource.canConstructInDedicatedWorker](https://developer.mozilla.org/en-US/docs/Web/API/MediaSource/canConstructInDedicatedWorker_static)
- [coi-serviceworker](https://github.com/gzuidhof/coi-serviceworker)
