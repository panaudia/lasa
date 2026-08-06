# Native Go client: feasibility study

*2026-08-05. A small feasibility study for a native desktop client
written in Go, replacing the previous-generation Swift/macOS app that
used ROC streaming and virtual device drivers. The proposed app
connects one or more OS input devices, streams each channel up as
`mono-object`, subscribes rendered sinks, and plays out to OS output
devices — with an HTML control UI. Two questions were researched
(library survey verified against upstream repos and issue trackers,
August 2026): how to reach OS audio devices from Go, and how to give
a Go app an HTML interface. Distribution and latency are addressed
at the end.*

## 0. Summary of conclusions

- **Feasible, and cleaner than the Swift/ROC approach**: no kernel or
  driver component, no ROC dependency, and the client imports the
  existing `lasa` Go packages (`wire`, `connect`, `client`, `ident`)
  directly — the protocol layer is already written.
- **Audio I/O: `gen2brain/malgo`** (miniaudio bindings) +
  **`hraban/opus`** (libopus). Multitrack capture via **one CoreAudio
  aggregate device** opened as a single N-channel stream, letting the
  OS handle inter-device clock drift.
- **UI: Wails v3** (native window + systray, generated TypeScript
  bindings, Vite frontend) — with a serious fallback of a localhost
  `net/http` server + small Vite/TS frontend + WebSocket, which
  reuses the in-progress TS browser client library almost verbatim
  and can later be wrapped in Wails unchanged.
- **Distribution**: one signed, notarized macOS `.app` whose only
  real content is a single Go binary with the UI baked in via
  `go:embed`. Static-link libopus; malgo needs nothing.
- **Latency**: expect roughly **20–40 ms less** combined ADC/DAC
  latency than a browser client, plus smoother uplink packet pacing
  (shallower server jitter buffer) and a cleaner embodiment of the
  hardware clock in the packet stream.
- **First milestone**: a 3–5 day audio POC with a hard loopback
  round-trip measurement, before committing to the full app. malgo's
  real CoreAudio latency is the only genuinely unproven number.

## 1. OS audio I/O from Go

### 1.1 Requirements

Enumerate and open one or more input devices simultaneously
(multitrack capture), stream each channel up as mono 48 kHz Opus in
5 ms (240-sample) frames; receive and play out to selected output
devices (stereo binaural; possibly multichannel ambisonic to an
interface). Callback-based, small buffers, float32, cgo acceptable.
macOS (Apple Silicon) primary; Windows/Linux nice-to-have.

### 1.2 Candidates surveyed

| Candidate | Maintenance (verified Aug 2026) | Platforms | Duplex | Multi-device | Callback / latency | Deps |
|---|---|---|---|---|---|---|
| **gen2brain/malgo** (miniaudio) | Active — v0.11.25, commits May 2026 | macOS/Win/Linux/BSD/iOS/Android | Yes (Playback/Capture/Duplex/Loopback) | Yes — repeated `InitDevice` on one `Context` | `DataProc` callback; `PeriodSizeInFrames`, LowLatency profile; `FormatF32` | cgo, **no external libs** (miniaudio vendored, header-only) |
| gordonklaus/portaudio | Barely maintained — last functional commit Feb 2025 | macOS/Win/Linux | Yes | Yes (stream per device) | Typed float32 callback; explicit latency params | cgo + **system PortAudio** (brew/pkg-config) — distribution burden |
| Direct CoreAudio (AUHAL) via cgo | No maintained wrapper exists | macOS only | Yes | Yes, full control | Best possible; you set `kAudioDevicePropertyBufferFrameSize` | All C glue is yours; ~1.5–3 weeks |
| ebitengine/oto v3 | Active | Cross-platform | **Playback only** — no capture | n/a | Pull-model `io.Reader` | Not suitable |
| RtAudio bindings | Dead (last commit 2020) | — | — | — | — | Not viable |
| jfreymuth/pulse | Alive; pure Go | Linux only | Two streams | Yes | Adequate; Pulse adds latency | Zero cgo — Linux fallback only |

Opus:

| Candidate | Status | Encode | Decode | 5 ms @ 48 kHz | Notes |
|---|---|---|---|---|---|
| **hraban/opus** (`gopkg.in/hraban/opus.v2`) | Very active — commits July 2026 | Yes | Yes | **Yes — validates 2.5/5/10/20/40/60 ms**, float32 | cgo + system libopus (pkg-config); `nolibopusfile` tag eases static linking. No multistream wrapper |
| pion/opus | Pure Go; decoder is the mature part | Not production-usable | Yes | Unproven | Wrong risk profile for a live encode path |

### 1.3 Recommendation

**malgo + hraban/opus.** malgo is the only actively maintained,
cross-platform, dependency-free option with everything required; its
CoreAudio backend drives AUHAL directly — the same thing a
hand-rolled layer would do. hraban/opus does encode and decode,
float32, and explicitly supports 5 ms frames. The uplink design
(N independent mono streams) sidesteps its missing multistream API;
a small multistream cgo shim would be needed only if the native
client should ever *subscribe* `ambi2`/`ambi3` (recording /
monitoring), and can be added later.

Fallbacks, in order: gordonklaus/portaudio (pleasant API,
battle-tested CoreAudio host, but near-dormant and the system-library
dependency hurts notarized distribution); hand-rolled AUHAL cgo layer
(justified only if malgo's measured latency disappoints).

### 1.4 Multitrack and clock drift — the important design point

Do **not** open N separate capture devices. Two independent CoreAudio
devices drift (e.g. 48000.3 vs 47999.8 Hz effective), and neither
malgo nor anything else in the Go ecosystem compensates. Instead open
**one aggregate device as a single N-channel capture stream** —
CoreAudio itself resamples secondary sub-devices against a chosen
clock master when drift correction is enabled. Two ways in:

1. **Zero-code**: the user builds the aggregate in Audio MIDI Setup;
   the app enumerates it via malgo as one N-channel device.
2. **Programmatic**: a ~150-line cgo shim calling
   `AudioHardwareCreateAggregateDevice` with
   `kAudioSubDeviceDriftCompensationKey = 1` on sub-devices, handing
   the resulting UID to malgo. Composes cleanly — malgo need not know
   the device is aggregate.

This replaces what the old virtual drivers were doing, with an
OS-supported mechanism. Truly separate unaggregated devices (or
Linux/Windows multitrack) would need per-device rings plus a slow
feedback resampler — real work; deferred.

### 1.5 Gotchas recorded from issue trackers

- **miniaudio period size is a hint.** One report (miniaudio #937)
  measured ~1323 frames of buffering despite requesting 128 — but on
  WASAPI-shared and PulseAudio, the two backends with fat OS mixers.
  CoreAudio is the good case (miniaudio sets the device buffer frame
  size directly; macOS floors at ~32 frames, so 240 is comfortable).
  Still: **measure round-trip latency in the POC before committing.**
  This is the single biggest risk with malgo.
- malgo callbacks hand over `[]byte` on the OS audio thread — cast
  with `unsafe.Slice`, keep the callback allocation-free
  (pre-allocated rings, no GC churn; the same discipline as the
  server hot path). cgo callback overhead is a few µs — fine at 5 ms.
- Selecting a non-default capture device requires setting
  `DeviceConfig.Capture.DeviceID` to a pointer of the enumerated
  `DeviceInfo.ID` (issues #52/#56) — fiddly but works.
- Use the callback (`DataProc`) API, not malgo's io wrapper (#60).
- hraban/opus needs pkg-config + libopus at build time (brew `opus`);
  static-link for distribution (§3).
- **macOS mic permission (TCC)**: a bundled app needs
  `NSMicrophoneUsageDescription`; a bare CLI inherits the terminal's
  grant — ship a real `.app` even in development demos.

### 1.6 POC plan (3–5 days)

Capture N channels @ 48 kHz float32 with 240-frame periods, encode
each channel to 5 ms mono Opus, decode + play stereo out.

- Day 1: malgo context / enumeration / capture callback; deinterleave
  to per-channel float32 rings.
- Day 2: per-channel encoders (5 ms, `AppOptRestrictedLowdelay`),
  decode path, playback device.
- Day 3: **loopback round-trip latency measurement**, buffer tuning,
  verify 240-frame periods actually granted on real hardware.
- Days 4–5 (buffer): aggregate-device testing with two physical
  inputs, TCC handling, glitch hardening.
- +2 days if programmatic aggregate creation is wanted in the POC.

## 2. HTML UI for the Go app

### 2.1 Candidates surveyed

| | Maintenance (verified Aug 2026) | Window vs tab | Go↔UI comms | TS support | macOS packaging | Binary size |
|---|---|---|---|---|---|---|
| **Wails v3 (beta)** | Very active; v3.0.0-beta.0 2026-08-02, API declared stable | Native window + built-in systray + multi-window | Generated typed bindings + typed events | Excellent (generated TS) | `wails3 package`, signing wizard, automated notarization | ~8–12 MB `.app` |
| Wails v2.13 | Stable line, July 2026 release | Native window | Reflection bindings + events | Good | Documented codesign/notarytool | ~8–12 MB |
| **localhost + Vite TS + WS** | Stdlib + Vite, all healthy | Browser tab (lifecycle/port handling needed) | WebSocket/SSE + REST | Native — reuses the TS client library directly | Plain codesign + notarytool on one binary | Smallest |
| localhost + htmx/templ | templ active | Browser tab | HTTP partials | Mostly no TS | Same | Small |
| webview_go | Dormant (bindings Aug 2024) | Bare native window | Manual `bind()` | None | DIY | ~5 MB |
| Lorca | Dead; needs installed Chrome | Chrome app-mode | DevTools protocol | None | DIY | Tiny + external Chrome |
| energye/energy | Active but CEF-bundled, docs largely Chinese | Native window | IPC | Weak | DIY | 100+ MB |
| Fyne / Gio (not HTML) | Healthy | Native | n/a | None | OK | — |

The thin-wrapper niche has consolidated on Wails; nothing significant
emerged 2024–2026. Fyne/Gio are fine projects but transfer none of
the web/TS investment.

### 2.2 Recommendation

**Wails v3.** Rationale:

1. **The TS investment compounds**: v3 generates TypeScript bindings
   and typed events from Go code (static source analysis, not
   reflection); the frontend is an ordinary Vite project, so types
   and utility code can be shared with the panaudia browser client.
2. **Matches the product shape**: built-in systray with an attached
   popup window — the menu-bar-audio-app pattern — plus native
   lifecycle (quit = quit; no orphaned tabs, no port juggling).
3. **Meters are a non-problem**: one typed event at 10–30 Hz carrying
   all channel levels, rendered to canvas in the webview.
4. **Best distribution story available** (§3).
5. Risk: the beta is days old (beta.4 on 2026-08-05). Mitigation:
   v2.13 is a mature same-architecture fallback with a migration
   path.

**Fallback worth taking seriously**: localhost `net/http` +
`go:embed` static files + small Vite/TS frontend + WebSocket for
meters, with `fyne.io/systray` (the maintained fork) for a tray icon
that opens the tab. Least code, zero framework risk, maximum reuse of
the TS client library. Costs "app-ness" (tab lifecycle, browser
chrome, port handling — bind `127.0.0.1:0` and open the resulting
URL). Crucially **not an either/or**: a Vite TS frontend built this
way drops into a Wails v3 shell almost unchanged, so starting with
the tab and wrapping in Wails later is a legitimate path.

Avoid: webview_go / Lorca (dormant/dead), energye (CEF bloat),
Fyne/Gio (no HTML, no skill reuse).

Effort to first working UI (device pickers + live meters +
connection status): **2–4 days** in Wails v3 (~0.5 day beta-friction
contingency included); ~1.5–2.5 days for the localhost variant.

## 3. Distribution

A macOS `.app` bundle — but the HTML/JS/CSS does not ship as loose
resource files. The built Vite `dist/` is embedded into the Go
executable with `go:embed`; Wails serves the embedded assets to the
WKWebView through its internal asset handler (the localhost variant
serves the same embed via `net/http`). The bundle is essentially:

```
Panaudia.app/Contents/
  MacOS/panaudia          ← Go binary, frontend embedded (~10 MB)
  Info.plist              ← incl. NSMicrophoneUsageDescription
  Resources/icon.icns
  (Frameworks/libopus.dylib — only if not statically linked)
```

- **Native deps**: malgo vendors miniaudio — nothing to bundle.
  libopus is the one external library: prefer static linking
  (`nolibopusfile` tag, link the `.a`) so the bundle stays a single
  file; else ship the dylib in `Frameworks/` with an `@rpath` fixup.
- **Signing**: Developer ID + hardened runtime, notarize with
  `notarytool`, staple, ship as DMG or zip. `wails3 package`
  automates this (signing wizard, notarization with retry); the
  localhost variant scripts it by hand (~a day of one-time setup).
- **TCC**: ship a real `.app` so macOS attributes the microphone
  permission to the app rather than the launching terminal.
- **App Store**: would additionally need the sandbox with
  `audio-input` and `network-client` entitlements — feasible,
  deferred; direct download is the norm for audio tools.
- **Updates**: nothing built in. Options: Homebrew cask, an in-UI
  update check linking to a download, or a Go self-update library.
- **Cross-platform**: the same embed-into-binary approach yields a
  Linux binary and Windows `.exe` from the same tree; only the
  bundle/signing layer is per-OS.

## 4. Latency versus the browser client

The audio path never touches the webview — the UI is control surface
only; audio is Go talking straight to CoreAudio. Compared against a
browser client:

**Where the browser spends time the native app won't:**

- *Capture*: `getUserMedia` audio crosses into the browser's capture
  service (separate process, IPC, own buffering) and — unless
  disabled — the WebRTC processing chain (AEC/AGC/NS) operating on
  10 ms blocks. Even tuned, capture rarely gets much under ~10–20 ms
  in Chrome.
- *Render-quantum mismatch*: AudioWorklets process in 128-frame
  quanta, and 240 is not a multiple of 128 — the browser client must
  ring-buffer between the worklet clock and the LASA frame clock in
  both directions, costing up to a quantum of adaptation each way
  plus scheduling slack.
- *Output*: `AudioContext.baseLatency` at `latencyHint:
  "interactive"` is commonly 5–10 ms on macOS Chrome, plus
  `outputLatency` to the device.

Realistic browser total (ADC→app + app→DAC): **~30–60 ms**, varying
by browser and subject to change under browser updates. Native path
with 240-frame CoreAudio periods: roughly one period buffer plus the
device safety offset and converter latency per side — **~10–15 ms
combined** on Apple Silicon, less with a good interface. Expected
saving: **~20–40 ms**, landing directly on total mouth-to-ear latency
since the network / jitter buffer / server tick are identical in both
cases.

**Smoothness — the subtler win.** The server's per-source jitter
buffer depth is sized by *arrival jitter*, not average latency. In
the browser, frames complete on the 128-sample worklet grid in an
irregular beat pattern, and the WebTransport datagram send crosses to
another thread through the event loop. The native app encodes and
sends adjacent to the CoreAudio callback — a realtime-priority thread
on the device's own clock — so packets leave at an almost perfectly
even 5 ms cadence, letting the server buffer run shallower before
adapting. The same applies in reverse on the downlink (playback
callback pulls on the DAC clock; the client-side jitter buffer sits
shallower for the same glitch rate). The stream's implicit clock
(seq, gapless — reconciled at the server's jitter buffer per the
one-clock design) is the ADC clock embodied directly, not a noisy
proxy through the browser's resampling layers.

This should also beat the previous ROC/virtual-driver setup: virtual
devices add a driver hop, and ROC carries its own FEC/jitter
machinery, whereas here the capture callback feeds the Opus encoder
directly.

## 5. Proposed shape and next step

One Go binary: `lasa` client packages for the MoQ / state / presence
plane → malgo capture (aggregate device, N channels) → per-channel
hraban/opus encoders publishing `mono-object` tracks → subscribe
`binaural` (later `ambi2`/`ambi3`, needing a small multistream shim)
→ malgo playback. Wails v3 (or the localhost+TS variant first) on
top for control and meters.

**Next step**: the §1.6 audio POC, gated on its day-3 loopback
latency measurement.

## Sources

Audio: [malgo](https://github.com/gen2brain/malgo) ·
[malgo API](https://pkg.go.dev/github.com/gen2brain/malgo) ·
[malgo issues](https://github.com/gen2brain/malgo/issues) ·
[miniaudio #937](https://github.com/mackron/miniaudio/issues/937) ·
[portaudio](https://github.com/gordonklaus/portaudio) ·
[hraban/opus](https://github.com/hraban/opus) ·
[hraban/opus API](https://pkg.go.dev/gopkg.in/hraban/opus.v2) ·
[pion/opus](https://github.com/pion/opus) ·
[pion/opus roadmap #9](https://github.com/pion/opus/issues/9) ·
[oto](https://github.com/ebitengine/oto) ·
[jfreymuth/pulse](https://github.com/jfreymuth/pulse) ·
[Apple: aggregate device settings](https://support.apple.com/guide/audio-midi-setup/set-aggregate-device-settings-ams094c7edb4/mac) ·
[Rogue Amoeba on drift correction](https://rogueamoeba.com/support/knowledgebase/?showArticle=Loopback-AggregateDeviceHandling)

UI: [Wails releases](https://github.com/wailsapp/wails/releases) ·
[v3 site / roadmap](https://v3.wails.io/) ·
[typed events PR #4161](https://github.com/wailsapp/wails/pull/4161) ·
[v3 systray docs](https://v3.wails.io/features/menus/systray/) ·
[v3 PKG/notarization PR #4472](https://github.com/wailsapp/wails/pull/4472) ·
[v3 signing guide](https://v3alpha.wails.io/guides/signing/) ·
[webview_go](https://github.com/webview/webview_go) ·
[Lorca](https://github.com/zserge/lorca) ·
[fyne.io/systray](https://github.com/fyne-io/systray) ·
[templ](https://github.com/a-h/templ)
