# DONE: Bluetooth HFP collapse & microphone selection

*Status: **DONE 2026-08-06** — all three candidate fixes below were
implemented the same day (Paul's ruling after the spatial-rendering
session closed):*

1. *`selectBestMicrophone` never opens a device: the priming open and
   the unknown-device sample-rate probe are removed; without
   permission it returns `permissionPending` and defers to the
   system default.*
2. *First-grant recovery in the test page: the unavoidable
   first-ever open lands on the default; if that proves Bluetooth
   (label or ≤16 kHz), the page re-selects with labels now readable,
   releases the BT mic, and re-opens the best non-Bluetooth device.
   Subsequent sessions never open a BT mic.*
3. *Post-hoc detection restored: `probeOutputDeviceSampleRate` ported
   into the LASA SDK (`player.ts`) + page banner (≤24 kHz ⇒ HFP
   suspected, with the old recovery advice), checked 1 s after a mic
   opens and every 10 s while audio is active.*

*Divergence from the verbatim copy recorded in the SDK's
PROVENANCE.md. The findings below are retained as the evidence that
drove the design.*

*(Original status: PARKED — investigated, not fixed, while rendering
issues took priority. The notes below record the old-vs-new
comparison so the fix pass could start from evidence.)*

## The symptom

AirPods as output: stereo collapses to mono (HFP) when using the LASA
test page; the old moq-test-page + old TS SDK preserved stereo on the
same machine. Once collapsed, AirPods hold HFP and don't recover
readily (old client's recovery advice: set Input to built-in in
System Settings, pods in case ~10 s, reconnect).

## Findings (2026-08-06 code comparison, old vs new)

**The old client fought this battle in June 2026 and encoded its
conclusions** (`moq-test-page/src/app.ts` "Mic picker" comment;
`panaudia-client.ts` `connect()` comment, both dated 2026-06-11):

- Pre-connect Bluetooth gating/auto-selection was REMOVED after
  cross-browser testing: Chrome self-manages a Bluetooth default,
  Firefox collapses regardless of mic choice, and on Safari the
  gating flow ITSELF triggered HFP.
- Resulting doctrine: never open a device for selection purposes.
  Mic dropdown is manual-only, populated only when permission is
  already granted (labels readable without opening). Bluetooth
  handling is warn-only. Exactly ONE mic open per session (the
  production getUserMedia with the user's explicit deviceId or
  unconstrained default).
- Detection moved POST-connect: `probeOutputDeviceSampleRate()` —
  output device at ≤24 kHz ⇒ HFP suspected ⇒ banner with
  browser-specific recovery advice.

**The new LASA test page reintroduced exactly what June removed, and
dropped the safety nets:**

1. "Auto (avoid Bluetooth)" → `selectBestMicrophone(true)`
   (`sdks/typescript-lasa/src/audio/microphone-selection.ts`, verbatim
   copy of the old module the old page had stopped driving this way)
   has two device-OPENING paths:
   - **Priming**: permission not granted ⇒ `getUserMedia({audio:true})`
     opens the DEFAULT device just to read labels — if AirPods are the
     macOS default input, that is the collapse, before selection runs.
   - **Probing**: any input whose label misses the keyword lists is
     OPENED to read its sample rate (≤16 kHz = HFP tell). AirPods
     renamed without "airpods" in the label would be probed = opened.
2. No post-collapse detection: the output-rate probe + HFP banner
   were not ported to the LASA page/SDK.
3. (Minor, likely not the trigger) graph ordering inverted: page
   creates+resumes the capture AudioContext BEFORE the mic opens; old
   opened mic first. Output-only contexts stay A2DP.

**Why old preserved stereo on the same machine (origin theory):** the
old page ALSO opened an unconstrained `getUserMedia({audio:true})`
(input level meter, on authenticate) and survived — so on the OLD
origin Chrome's unconstrained opens landed on the built-in mic.
Chrome remembers permission state and mic preference PER-ORIGIN
(chosen at the first prompt). The LASA page is a NEW origin
(dev.panaudia.com:<port> — port counts): permission starts ungranted,
so the first Auto click primes against Chrome's preselection (the OS
default; macOS flips default input to AirPods on connect). An
"Allow this time" grant makes the priming re-run EVERY session.

## Diagnosis step (do this first when resuming)

`selectBestMicrophone(true)` already logs `[MicSelection]` lines
(enumerated labels/types, probes, selection) and the page logs
"auto-selected mic" + "mic settings — … Hz" (16000 Hz = HFP tell).
From a collapsing session, the console discriminates:
- no `[MicSelection]` enumeration before collapse ⇒ priming path
  (permission wasn't granted);
- a probe line naming the AirPods ⇒ probe path;
- clean builtin selection yet still collapsing ⇒ theory falsified,
  look elsewhere (e.g. outside-page opens).
Also check chrome://settings/content/microphone for the origin's
remembered device.

## Candidate fixes (in order of how directly they follow June's lessons)

1. Auto path must never open a device it hasn't already chosen:
   prime via a deviceId-constrained open of a label-classified non-BT
   candidate, or skip priming entirely and fall back to
   builtin-by-label.
2. Drop the unknown-device sample-rate probe (accept a worse pick
   over an HFP trigger).
3. Port the old post-connect HFP detection (output device rate ≤24 kHz
   ⇒ banner + recovery advice) to the LASA page/SDK so a collapse is
   at least visible.
