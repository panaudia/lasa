/**
 * LASA test page — a browser instrument for @panaudia/lasa-client
 * against a running LASA space. Every panel exercises a subsystem: the
 * multi-entity connection, per-entity capture pipelines (tone or mic)
 * with live pose (the SAB freshness path under a finger), worklet
 * playout with its diagnostics, the state plane, and presence.
 *
 * Entities are declared BEFORE connecting: the LASA Connection Config
 * rides CLIENT_SETUP, so the entity set is fixed per connection.
 */

import {
  LasaClient,
  Store,
  loudnessToDBFS,
  LOUDNESS_SILENT,
  selectBestMicrophone,
  classifyByLabel,
  micPermissionGranted,
  probeOutputDeviceSampleRate,
  PresenceRoster,
  type CaptureHandle,
  type SinkPlayer,
  type Pose,
  type LasaEntity,
} from '@panaudia/lasa-client';

// ---------- tiny DOM helpers ----------

function $<T extends HTMLElement>(id: string): T {
  return document.getElementById(id) as T;
}

const logEl = $<HTMLDivElement>('log');
const LOG_CAP = 30_000; // chars — unbounded growth churns the DOM and heap
function log(line: string): void {
  const t = new Date().toISOString().slice(11, 19);
  let text = `${t}  ${line}\n` + (logEl.textContent ?? '');
  if (text.length > LOG_CAP) text = text.slice(0, LOG_CAP);
  logEl.textContent = text;
  console.log('[page]', line);
}

// ---------- page state ----------

interface EntityUI {
  id: string;
  name: string;
  pose: Pose;
  seq: bigint; // pose-only publish sequence (non-capturing entities)
  row: HTMLDivElement;
  srcSel: HTMLSelectElement;
  captureBtn: HTMLButtonElement;
  listenBtn: HTMLButtonElement;
  removeBtn: HTMLButtonElement;
  capture?: CaptureHandle;
  captureNodes?: { osc?: OscillatorNode; micStream?: MediaStream };
  player?: SinkPlayer;
}

let client: LasaClient | null = null;
let store: Store | null = null;
let captureCtx: AudioContext | null = null;
const entities = new Map<string, EntityUI>();
// Presence roster: the SDK applies the §7 receive rules (generation
// discard, latest-wins by group id); we only read snapshots.
let roster: PresenceRoster | null = null;
let selectedEntity: string | null = null;

const WORLD_HALF = 5; // map shows ±5 m

// ---------- isolation banner ----------

const iso = $<HTMLSpanElement>('isolation');
if (globalThis.crossOriginIsolated) {
  iso.textContent = 'cross-origin isolated ✓ (SAB available)';
} else {
  iso.textContent = 'NOT cross-origin isolated — capture/playout will refuse';
  iso.classList.add('bad');
}

// ---------- HFP banner ----------

// The output-device rate probe is the only in-browser signal for a
// Bluetooth headset collapsed to mono (HFP): the device reports a
// 16–24 kHz rate. HFP collapse happens in the OS below every Web Audio
// observable, so detection is post-hoc by design (the old client's
// 2026-06-11 finding: pre-emptive gating flows trigger the collapse
// themselves). Checked shortly after a mic opens and every 10 s while
// audio is active.
const hfpBanner = $<HTMLDivElement>('hfp-banner');
function hfpAdvice(): string {
  if (/firefox/i.test(navigator.userAgent)) {
    return 'Firefox forces a Bluetooth headset to mono whenever any microphone is in use — use wired output or another browser.';
  }
  return 'Set Input to the built-in microphone in System Settings → Sound, put the headset in its case for ~10 s, then reconnect.';
}
let hfpCheckBusy = false;
async function checkHfp(): Promise<void> {
  if (hfpCheckBusy) return;
  const active = [...entities.values()].some((e) => e.captureNodes?.micStream || e.player);
  if (!active) {
    hfpBanner.hidden = true;
    return;
  }
  hfpCheckBusy = true;
  try {
    const rate = await probeOutputDeviceSampleRate();
    const stuck = rate !== null && rate <= 24000;
    if (stuck) {
      hfpBanner.textContent = `Output device is at ${rate} Hz — Bluetooth mono (HFP) suspected. ${hfpAdvice()}`;
    }
    hfpBanner.hidden = !stuck;
  } finally {
    hfpCheckBusy = false;
  }
}
window.setInterval(() => void checkHfp(), 10_000);

// ---------- microphone selection ----------

// Source select values: 'tone' | 'mic-auto' | 'mic:<deviceId>'.
// Device labels are only available once mic permission is granted; the
// lists refresh after the first successful capture and on devicechange.

let knownMics: { deviceId: string; label: string }[] = [];

function populateSourceSelect(sel: HTMLSelectElement): void {
  const current = sel.value;
  sel.innerHTML = '';
  const add = (value: string, text: string) => {
    const o = document.createElement('option');
    o.value = value;
    o.textContent = text;
    sel.append(o);
  };
  add('tone', 'Tone');
  add('mic-auto', 'Mic — auto (best non-Bluetooth)');
  for (const m of knownMics) {
    add(`mic:${m.deviceId}`, `Mic — ${m.label} [${classifyByLabel(m.label)}]`);
  }
  if ([...sel.options].some((o) => o.value === current)) sel.value = current;
}

async function refreshMicOptions(): Promise<void> {
  // Without permission, labels are empty and deviceIds unstable — the
  // module's rule: never open a mic just to read labels.
  if (!(await micPermissionGranted())) return;
  const devices = await navigator.mediaDevices.enumerateDevices();
  knownMics = devices
    .filter((d) => d.kind === 'audioinput' && d.deviceId !== 'default')
    .map((d) => ({ deviceId: d.deviceId, label: d.label || '(unlabelled)' }));
  for (const e of entities.values()) populateSourceSelect(e.srcSel);
}

navigator.mediaDevices?.addEventListener?.('devicechange', () => void refreshMicOptions());

// ---------- entity rows ----------

const entitiesEl = $<HTMLDivElement>('entities');

function addEntityRow(id: string, name: string): void {
  if (entities.has(id)) {
    log(`entity ${id} already exists`);
    return;
  }
  const row = document.createElement('div');
  row.className = 'entity';

  const label = document.createElement('span');
  label.className = 'eid';
  label.textContent = id;

  const srcSel = document.createElement('select');
  populateSourceSelect(srcSel);

  const captureBtn = document.createElement('button');
  captureBtn.textContent = 'Start capture';
  captureBtn.disabled = true;
  captureBtn.onclick = () => void toggleCapture(id);

  const listenBtn = document.createElement('button');
  listenBtn.className = 'secondary';
  listenBtn.textContent = 'Listen';
  listenBtn.disabled = true;
  listenBtn.onclick = () => void toggleListen(id);

  const removeBtn = document.createElement('button');
  removeBtn.className = 'secondary';
  removeBtn.textContent = 'Remove';
  removeBtn.onclick = () => {
    if (client) return; // set is fixed once connected
    entities.delete(id);
    row.remove();
  };

  row.append(label, srcSel, captureBtn, listenBtn, removeBtn);
  entitiesEl.append(row);

  const idx = entities.size;
  entities.set(id, {
    id,
    name,
    pose: { x: (idx % 5) - 2, y: 1 + Math.floor(idx / 5), z: 0, yaw: 0, pitch: 0, roll: 0 },
    seq: 0n,
    row,
    srcSel,
    captureBtn,
    listenBtn,
    removeBtn,
  });
}

$<HTMLButtonElement>('add-entity').onclick = () => {
  const id = $<HTMLInputElement>('new-entity-id').value.trim();
  const name = $<HTMLInputElement>('new-entity-name').value.trim() || id;
  if (!id) return;
  addEntityRow(id, name);
  $<HTMLInputElement>('new-entity-id').value = '';
  $<HTMLInputElement>('new-entity-name').value = '';
};

// Default the server URL to the page's own hostname (127.0.0.1 on the
// plain-HTTP route, dev.panaudia.com on the wildcard-cert route).
$<HTMLInputElement>('url').value = `https://${location.hostname}:4443/lasa`;

// Two starter entities so the page is useful immediately.
const suffix = Math.random().toString(36).slice(2, 6);
$<HTMLInputElement>('client-id').value = `page-${suffix}`;
addEntityRow(`speaker-${suffix}`, 'Speaker');
addEntityRow(`listener-${suffix}`, 'Listener');
$<HTMLButtonElement>('add-entity').disabled = false;
// If mic permission is already granted, labelled device lists are
// available immediately.
void refreshMicOptions();

// ---------- connect / disconnect ----------

const connectBtn = $<HTMLButtonElement>('connect');
const disconnectBtn = $<HTMLButtonElement>('disconnect');
const statusEl = $<HTMLSpanElement>('conn-status');

connectBtn.onclick = () => void connect();
disconnectBtn.onclick = () => void disconnect();

async function connect(): Promise<void> {
  if (client) return;
  connectBtn.disabled = true;
  try {
    const spaceId = $<HTMLInputElement>('space').value.trim();
    // Entities are fixed at CLIENT_SETUP, and so are their initial
    // poses — without this the server homes every entity at the origin
    // until its first drag, and the map draws a fiction.
    const entityDefs: LasaEntity[] = [...entities.values()].map((e) => ({
      id: e.id,
      name: e.name,
      'initial-pose': {
        position: { x: e.pose.x, y: e.pose.y, z: e.pose.z },
        attitude: { yaw: e.pose.yaw, pitch: e.pose.pitch, roll: e.pose.roll },
      },
    }));
    const ticket = $<HTMLTextAreaElement>('ticket').value.trim();
    const certHash = $<HTMLInputElement>('cert-hash').value.trim();

    client = await LasaClient.connect({
      url: $<HTMLInputElement>('url').value.trim(),
      spaceId,
      clientId: $<HTMLInputElement>('client-id').value.trim(),
      ticket: ticket || undefined,
      entities: entityDefs,
      serverCertificateHashBase64: certHash || undefined,
    });
    log(`connected (${entityDefs.length} entities)`);
    statusEl.textContent = 'connected';
    statusEl.classList.add('on');

    store = await Store.create(['lasa.', 'app.']);
    client.syncState(store);
    roster = await client.subscribeRoster();

    for (const e of entities.values()) {
      e.captureBtn.disabled = false;
      e.listenBtn.disabled = false;
      e.removeBtn.disabled = true;
    }
    $<HTMLButtonElement>('add-entity').disabled = true;
    $<HTMLButtonElement>('state-set').disabled = false;
    $<HTMLButtonElement>('state-clear').disabled = false;
    disconnectBtn.disabled = false;
  } catch (e) {
    log(`connect failed: ${String(e)}`);
    client = null;
    connectBtn.disabled = false;
  }
}

async function disconnect(): Promise<void> {
  if (!client) return;
  disconnectBtn.disabled = true;
  try {
    await client.close();
  } catch (e) {
    log(`close error: ${String(e)}`);
  }
  client = null;
  store = null;
  for (const e of entities.values()) {
    stopCaptureNodes(e);
    e.capture = undefined;
    e.player = undefined;
    e.captureBtn.textContent = 'Start capture';
    e.listenBtn.textContent = 'Listen';
    e.captureBtn.disabled = true;
    e.listenBtn.disabled = true;
    e.removeBtn.disabled = false;
  }
  if (captureCtx) {
    await captureCtx.close().catch(() => {});
    captureCtx = null;
  }
  roster = null;
  $<HTMLButtonElement>('add-entity').disabled = false;
  $<HTMLButtonElement>('state-set').disabled = true;
  $<HTMLButtonElement>('state-clear').disabled = true;
  statusEl.textContent = 'disconnected';
  statusEl.classList.remove('on');
  connectBtn.disabled = false;
  log('disconnected');
}

// ---------- capture ----------

async function toggleCapture(id: string): Promise<void> {
  const e = entities.get(id);
  if (!e || !client) return;
  if (e.capture) {
    await client.stopCapture(id).catch((err) => log(`stopCapture: ${String(err)}`));
    stopCaptureNodes(e);
    e.capture = undefined;
    e.captureBtn.textContent = 'Start capture';
    log(`${id}: capture stopped`);
    return;
  }
  e.captureBtn.disabled = true;
  try {
    captureCtx ??= new AudioContext({ sampleRate: 48000 });
    await captureCtx.resume();

    let source: AudioNode;
    const sel = e.srcSel.value;
    if (sel === 'mic-auto' || sel.startsWith('mic:')) {
      const auto = sel === 'mic-auto';
      let deviceId: string | undefined;
      if (auto) {
        // Selection never opens a device (opening a Bluetooth mic IS
        // the HFP trigger). On a true first use (permission pending)
        // it defers to the default; the recovery below switches away
        // if that default proves Bluetooth.
        const pick = await selectBestMicrophone(true);
        deviceId = pick.deviceId;
        log(
          `${id}: auto-selected mic — ${pick.label} [${pick.type}]` +
            (pick.switchedFromBluetooth ? ' (switched away from a Bluetooth default)' : '') +
            (pick.permissionPending ? ' (first use: system default until permission granted)' : '')
        );
      } else {
        deviceId = sel.slice(4);
      }
      let micStream: MediaStream = await openMic(deviceId);
      if (auto) {
        // First-grant recovery: if the default we were forced to open
        // turned out Bluetooth, permission now exists — re-select with
        // labels and switch. The Bluetooth open may already have
        // flipped the headset to HFP for this session (the banner
        // below will say so), but every future session avoids it.
        const track = micStream.getAudioTracks()[0];
        const openedBluetooth =
          track !== undefined &&
          (classifyByLabel(track.label) === 'bluetooth' || (track.getSettings().sampleRate ?? 48000) <= 16000);
        if (openedBluetooth) {
          const repick = await selectBestMicrophone(true);
          if (repick.deviceId && repick.type !== 'bluetooth') {
            for (const t of micStream.getTracks()) t.stop();
            micStream = await openMic(repick.deviceId);
            log(`${id}: default mic was Bluetooth (${track!.label}) — switched to ${repick.label} [${repick.type}]`);
          } else {
            log(`${id}: WARNING — Bluetooth mic in use (${track!.label}); no non-Bluetooth mic found. Stereo output may collapse to mono.`);
          }
        }
      }
      source = captureCtx.createMediaStreamSource(micStream);
      e.captureNodes = { micStream };
      // Surface what the browser actually configured — the input path
      // is otherwise unobservable (`latency` is seconds where reported).
      const s = micStream.getAudioTracks()[0]?.getSettings() as
        | (MediaTrackSettings & { latency?: number })
        | undefined;
      if (s) {
        log(
          `${id}: mic settings — latency ${s.latency !== undefined ? (s.latency * 1000).toFixed(1) + 'ms' : 'n/a'}, ` +
            `${s.sampleRate ?? '?'}Hz, ch ${s.channelCount ?? '?'}, ec ${String(s.echoCancellation)}, ns ${String(s.noiseSuppression)}, agc ${String(s.autoGainControl)}`
        );
      }
    } else {
      // Staggered tone per entity so simultaneous captures are audibly
      // distinct. An octave up from the original 220 base — a higher
      // sine gives somewhat better ILD localisation without the
      // annoyance of a pulsed/harmonic-rich probe signal.
      const freq = 440 * (1 + [...entities.keys()].indexOf(id));
      const osc = new OscillatorNode(captureCtx, { frequency: freq });
      const gain = new GainNode(captureCtx, { gain: 0.3 });
      osc.connect(gain);
      osc.start();
      source = gain;
      e.captureNodes = { osc };
    }

    e.capture = await client.startCapture(id, { source, pose: e.pose });
    e.captureBtn.textContent = 'Stop capture';
    log(`${id}: capturing (${e.srcSel.value})`);
    // Permission is granted now — device labels are available.
    void refreshMicOptions();
    // A freshly-opened mic is the HFP trigger moment: check the output
    // route once the OS has had a beat to flip it.
    if (e.captureNodes?.micStream) window.setTimeout(() => void checkHfp(), 1000);
  } catch (err) {
    log(`${id}: capture failed: ${String(err)}`);
    stopCaptureNodes(e);
  } finally {
    e.captureBtn.disabled = false;
  }
}

/** Open a microphone with the page's capture constraints (exact device when given). */
function openMic(deviceId: string | undefined): Promise<MediaStream> {
  return navigator.mediaDevices.getUserMedia({
    audio: {
      channelCount: 1,
      sampleRate: 48000,
      echoCancellation: false,
      noiseSuppression: false,
      autoGainControl: false,
      latency: { ideal: 0.005 },
      ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
    } as MediaTrackConstraints,
  });
}

function stopCaptureNodes(e: EntityUI): void {
  try {
    e.captureNodes?.osc?.stop();
  } catch {
    // already stopped
  }
  for (const track of e.captureNodes?.micStream?.getTracks() ?? []) track.stop();
  e.captureNodes = undefined;
}

// ---------- playout ----------

async function toggleListen(id: string): Promise<void> {
  const e = entities.get(id);
  if (!e || !client) return;
  if (e.player) {
    await client.stopSink(id).catch((err) => log(`stopSink: ${String(err)}`));
    e.player = undefined;
    e.listenBtn.textContent = 'Listen';
    log(`${id}: playout stopped`);
    return;
  }
  e.listenBtn.disabled = true;
  try {
    e.player = await client.playSink(id, 'binaural');
    e.listenBtn.textContent = 'Stop listening';
    log(`${id}: playing binaural sink`);
    // Jitter-debug: SUMMARISED buffer-discontinuity reporting (2 s
    // cadence). Per-event logging is banned here: in a bad state the
    // buffer emits hundreds of events/min, and logging each one grows
    // the console/heap, lengthens GC pauses, and so CAUSES more events
    // — a feedback loop observed live 2026-08-06.
    let seenEvents = 0;
    let polls = 0;
    const counts = new Map<string, number>();
    let fillMin = Infinity;
    let fillMax = -Infinity;
    const player = e.player;
    const evPoll = setInterval(() => {
      if (e.player !== player) {
        clearInterval(evPoll);
        return;
      }
      const events = player.getEvents();
      for (; seenEvents < events.length; seenEvents++) {
        const ev = events[seenEvents]!;
        counts.set(ev.kind, (counts.get(ev.kind) ?? 0) + 1);
        const fillMs = ev.fillFrames / 48;
        if (fillMs < fillMin) fillMin = fillMs;
        if (fillMs > fillMax) fillMax = fillMs;
      }
      if (events.length >= 490 && seenEvents > 400) {
        // Ring is about to rotate; resync rather than double-count.
        seenEvents = 0;
        player.clearEvents();
      }
      if (++polls % 8 === 0 && counts.size > 0) {
        const parts = [...counts.entries()].map(([k, n]) => `${k}=${n}`).join(' ');
        log(`${id}: [jbuf] 2s: ${parts} fill ${fillMin.toFixed(1)}–${fillMax.toFixed(1)}ms`);
        counts.clear();
        fillMin = Infinity;
        fillMax = -Infinity;
      }
    }, 250);
  } catch (err) {
    log(`${id}: playSink failed: ${String(err)}`);
  } finally {
    e.listenBtn.disabled = false;
  }
}

// ---------- pose: map + yaw ----------

const map = $<HTMLCanvasElement>('map');
const mapCtx = map.getContext('2d')!;
const yawSlider = $<HTMLInputElement>('yaw');
const yawEntityEl = $<HTMLSpanElement>('yaw-entity');
let dragging: string | null = null;
let poseSendTimer = 0;

// Top-down view of the engine frame (x forward, y left, z up):
// screen-up = +x (forward), screen-left = +y (left). Positive yaw is
// anticlockwise about z, so it sweeps anticlockwise on screen too.
function worldToCanvas(x: number, y: number): [number, number] {
  const s = map.width / (2 * WORLD_HALF);
  return [map.width / 2 - y * s, map.height / 2 - x * s];
}

function canvasToWorld(cx: number, cy: number): [number, number] {
  const s = map.width / (2 * WORLD_HALF);
  return [-(cy - map.height / 2) / s, -(cx - map.width / 2) / s];
}

function canvasPos(ev: PointerEvent): [number, number] {
  const r = map.getBoundingClientRect();
  return [((ev.clientX - r.left) / r.width) * map.width, ((ev.clientY - r.top) / r.height) * map.height];
}

map.onpointerdown = (ev) => {
  const [cx, cy] = canvasPos(ev);
  for (const e of entities.values()) {
    const [ex, ey] = worldToCanvas(e.pose.x, e.pose.y);
    if ((cx - ex) ** 2 + (cy - ey) ** 2 < 14 ** 2) {
      dragging = e.id;
      selectedEntity = e.id;
      yawSlider.disabled = false;
      yawSlider.value = String(e.pose.yaw);
      yawEntityEl.textContent = `yaw of ${e.id}`;
      map.setPointerCapture(ev.pointerId);
      return;
    }
  }
};

map.onpointermove = (ev) => {
  if (!dragging) return;
  const e = entities.get(dragging);
  if (!e) return;
  const [cx, cy] = canvasPos(ev);
  const [wx, wy] = canvasToWorld(cx, cy);
  e.pose.x = Math.max(-WORLD_HALF, Math.min(WORLD_HALF, wx));
  e.pose.y = Math.max(-WORLD_HALF, Math.min(WORLD_HALF, wy));
  schedulePoseSend(e);
};

map.onpointerup = () => {
  dragging = null;
};

yawSlider.oninput = () => {
  const e = selectedEntity ? entities.get(selectedEntity) : undefined;
  if (!e) return;
  e.pose.yaw = Number(yawSlider.value);
  schedulePoseSend(e);
};

/**
 * Send the entity's pose: capturing entities via the SAB pose cell
 * (freshest-wins, wait-free), others as pose-only mono-object packets.
 * Throttled to ~30/s per gesture stream.
 */
function schedulePoseSend(e: EntityUI): void {
  if (e.capture) {
    e.capture.setPose(e.pose); // no throttle needed — it's a seqlock write
    return;
  }
  if (poseSendTimer) return;
  poseSendTimer = window.setTimeout(() => {
    poseSendTimer = 0;
    void sendPoseOnly(e);
  }, 33);
}

async function sendPoseOnly(e: EntityUI): Promise<void> {
  if (!client) return;
  try {
    const pub = await client.entity(e.id, 1000);
    await pub.publish(e.seq++, { pose: e.pose });
  } catch (err) {
    log(`${e.id}: pose publish failed: ${String(err)}`);
  }
}

// ---------- map rendering ----------

function drawMap(): void {
  const w = map.width;
  mapCtx.clearRect(0, 0, w, w);

  // Grid: 1 m lines.
  mapCtx.strokeStyle = '#242830';
  mapCtx.lineWidth = 1;
  for (let m = -WORLD_HALF; m <= WORLD_HALF; m++) {
    const [cx] = worldToCanvas(m, 0);
    const [, cy] = worldToCanvas(0, m);
    mapCtx.beginPath();
    mapCtx.moveTo(cx, 0);
    mapCtx.lineTo(cx, w);
    mapCtx.moveTo(0, cy);
    mapCtx.lineTo(w, cy);
    mapCtx.stroke();
  }

  // Axis labels: the engine frame as seen from above.
  mapCtx.fillStyle = '#4a5261';
  mapCtx.font = '10px system-ui';
  mapCtx.textAlign = 'center';
  mapCtx.fillText('forward +x', w / 2, 12);
  mapCtx.textAlign = 'left';
  mapCtx.fillText('left +y', 4, w / 2 - 4);
  mapCtx.textAlign = 'start';

  const drawEntity = (id: string, pose: Pose, loudness: number, own: boolean) => {
    const [cx, cy] = worldToCanvas(pose.x, pose.y);
    const speaking = loudness !== LOUDNESS_SILENT;
    mapCtx.beginPath();
    mapCtx.arc(cx, cy, own ? 10 : 8, 0, Math.PI * 2);
    mapCtx.fillStyle = own ? '#4fb6a5' : '#5a6373';
    mapCtx.globalAlpha = speaking ? 1 : 0.55;
    mapCtx.fill();
    mapCtx.globalAlpha = 1;
    // Yaw tick (heading): facing is world (cos yaw, sin yaw) — +x at
    // yaw 0 (screen-up), swinging anticlockwise for positive yaw.
    mapCtx.strokeStyle = '#d8dde5';
    mapCtx.beginPath();
    mapCtx.moveTo(cx, cy);
    mapCtx.lineTo(cx - 14 * Math.sin(pose.yaw), cy - 14 * Math.cos(pose.yaw));
    mapCtx.stroke();
    mapCtx.fillStyle = '#aeb6c2';
    mapCtx.font = '11px system-ui';
    const level = speaking ? ` ${loudnessToDBFS(loudness).toFixed(0)}dB` : '';
    mapCtx.fillText(`${id}${level}`, cx + 12, cy - 10);
  };

  // Presence roster first (everyone), own entities on top from local pose.
  for (const entry of roster?.snapshot() ?? []) {
    if (!entities.has(entry.id)) drawEntity(entry.id, entry.pose, entry.loudness, false);
  }
  for (const e of entities.values()) {
    const remote = roster?.snapshot().find((r) => r.id === e.id);
    drawEntity(e.id, e.pose, remote?.loudness ?? LOUDNESS_SILENT, true);
  }

  requestAnimationFrame(drawMap);
}
requestAnimationFrame(drawMap);

// ---------- state panel ----------

$<HTMLButtonElement>('state-set').onclick = () => void writeStateOp('set');
$<HTMLButtonElement>('state-clear').onclick = () => void writeStateOp('clear');

async function writeStateOp(kind: 'set' | 'clear'): Promise<void> {
  if (!client) return;
  const key = $<HTMLInputElement>('state-key').value.trim();
  if (!key) return;
  try {
    if (kind === 'set') {
      const value = new TextEncoder().encode($<HTMLInputElement>('state-value').value);
      await client.writeState({ kind: 'set', key, value, seq: 0n });
    } else {
      await client.writeState({ kind: 'clear', key, seq: 0n });
    }
    log(`state ${kind}: ${key}`);
  } catch (e) {
    log(`state ${kind} failed: ${String(e)}`);
  }
}

const stateTable = $<HTMLDivElement>('state-table');
const valueDecoder = new TextDecoder();
setInterval(() => {
  if (!store) {
    stateTable.textContent = '';
    return;
  }
  const rows: string[] = [];
  for (const [key, kv] of store) {
    let text = valueDecoder.decode(kv.value);
    if (text.length > 80) text = text.slice(0, 77) + '…';
    rows.push(`<tr><td>${escapeHtml(key)}</td><td>${escapeHtml(text)}</td></tr>`);
  }
  stateTable.innerHTML = `<table>${rows.join('')}</table>`;
}, 500);

function escapeHtml(s: string): string {
  return s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c]!);
}

// ---------- diagnostics ----------

const diagEl = $<HTMLDivElement>('diagnostics');
setInterval(() => {
  const lines: string[] = [];
  for (const e of entities.values()) {
    if (e.capture && client) {
      const ovf = client.getCaptureOverflows(e.id);
      if (ovf) lines.push(`${e.id} capture: RING OVERFLOWS ${ovf} (dropped quanta = upstream gaps)`);
    }
    if (!e.player) continue;
    const s = e.player.getJitterStats();
    const b = e.player.getTapB();
    const g = e.player.getAudioGraphReport();
    if (!s) {
      lines.push(`${e.id}: waiting for stats…`);
      continue;
    }
    const tap = b
      ? `TapB rms ${(b.rmsL + b.rmsR).toFixed(3)} corr ${b.correlation.toFixed(2)} side ${b.sideRms.toFixed(3)}`
      : 'TapB —';
    lines.push(
      `${e.id}: fill ${s.fillMs.toFixed(1)}ms  sp ${s.setpointMs.toFixed(1)}  wl ${s.widthLowMs.toFixed(1)}  wh ${s.widthHighMs.toFixed(1)}  ` +
        `rate ${s.ratePerSec.toFixed(2)}/s${s.frozen ? '  FROZEN' : ''}  ` +
        `und ${s.underruns}  ovr ${s.overruns}  lap ${s.laps}  trim ${s.trims}  ins/drop ${s.samplesInserted}/${s.samplesDropped}  |  ${tap}  |  ` +
        `dev base ${g.context.baseLatencyMs?.toFixed(1) ?? '?'}ms out ${g.context.outputLatencyMs?.toFixed(1) ?? '?'}ms`
    );
    const ing = client?.getSinkIngress(e.id);
    if (ing) {
      lines.push(
        `${e.id} ingress: rx ${ing.received}  gaps ${ing.gapEvents} (lost ${ing.lostFrames} frames = ${(ing.lostFrames * 5).toFixed(0)}ms)  ` +
          `reorder ${ing.reordered}  decode-err ${ing.decodeErrors}`
      );
    }
  }
  diagEl.textContent = lines.join('\n') || 'no active playout';
}, 500);
