/**
 * The smallest useful LASA client.
 *
 * One entity per tab: the microphone goes up as that entity's source,
 * the space's binaural render of everything else comes back down as
 * its sink, and presence plus the base-profile state give the roster
 * its names and positions. Everything here is the public package
 * surface; see the guides in ../../docs for each part.
 */
import {
  LasaClient,
  PresenceRoster,
  Store,
  baseView,
  loudnessToDBFS,
  type CaptureHandle,
  type SinkPlayer,
  type StateSync,
} from '@panaudia/lasa-client';

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const form = $<HTMLFormElement>('join-form');
const joinButton = $<HTMLButtonElement>('join');
const leaveButton = $<HTMLButtonElement>('leave');
const status = $<HTMLSpanElement>('status');
const rosterBody = $<HTMLTableSectionElement>('roster');

interface Session {
  client: LasaClient;
  entityId: string;
  ctx: AudioContext;
  capture: CaptureHandle;
  player: SinkPlayer;
  roster: PresenceRoster;
  sync: StateSync;
  view: ReturnType<typeof baseView>;
}
let session: Session | null = null;

function setStatus(text: string): void {
  status.textContent = text;
}

// Everyone joins on a circle so two tabs are audibly apart. LASA
// axes (docs/coordinates.md): x forward, y left, z up, so the floor
// is the (x, y) plane. Metres; the pose's yaw faces the origin, as
// angle − π rather than angle + π because the wire clamps angles
// outside (−π, π] instead of wrapping them.
function poseOnCircle(): { x: number; y: number; z: number; yaw: number; pitch: number; roll: number } {
  const angle = Math.random() * Math.PI * 2;
  return { x: 2 * Math.cos(angle), y: 2 * Math.sin(angle), z: 0, yaw: angle - Math.PI, pitch: 0, roll: 0 };
}

async function join(): Promise<void> {
  const clientId = $<HTMLInputElement>('client').value.trim();
  const entityId = `${clientId}-voice`;
  const certHash = $<HTMLInputElement>('cert-hash').value.trim();
  setStatus('connecting');

  // The Connection Config declares this connection's entities up front;
  // the server subscribes each entity's source track in return.
  const client = await LasaClient.connect({
    url: $<HTMLInputElement>('url').value.trim(),
    spaceId: $<HTMLInputElement>('space').value.trim(),
    clientId,
    entities: [{ id: entityId, name: clientId }],
    serverCertificateHashBase64: certHash || undefined,
    onClosed: (info) => {
      setStatus(`closed${info.reason ? `: ${info.reason}` : ''}`);
      void teardown();
    },
  });

  try {
    // Capture: a 48 kHz context is required (the wire format is Opus at
    // 48 kHz in 5 ms frames). Echo cancellation and the rest are off:
    // this is a spatial mix, not a phone call.
    const ctx = new AudioContext({ sampleRate: 48000 });
    const mic = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false },
    });
    const capture = await client.startCapture(entityId, {
      source: ctx.createMediaStreamSource(mic),
      pose: poseOnCircle(),
    });

    // Playout: the space renders a binaural mix for this entity.
    const player = await client.playSink(entityId, 'binaural');

    // Presence: ids, poses and loudness, at presence rate.
    const roster = await client.subscribeRoster(render);

    // State: the base profile, for names. The view reads the store.
    const store = await Store.create(['lasa.']);
    const sync = client.syncState(store);
    const view = baseView(store);
    await sync.settled();

    session = { client, entityId, ctx, capture, player, roster, sync, view };
    joinButton.disabled = true;
    leaveButton.disabled = false;
    setStatus(`in the space as ${entityId}`);
    render();
  } catch (e) {
    await client.close().catch(() => {});
    throw e;
  }
}

function render(): void {
  if (!session) return;
  const { roster, view, entityId } = session;
  const rows = roster
    .snapshot()
    .sort((a, b) => (a.id < b.id ? -1 : 1))
    .map((e) => {
      const name = view.entity(e.id)?.name ?? '';
      const db = loudnessToDBFS(e.loudness);
      const level = db === null ? 'silent' : `${db.toFixed(0)} dBFS`;
      const cls = e.id === entityId ? ' class="me"' : '';
      return `<tr${cls}><td>${e.id}</td><td>${name}</td><td>${e.pose.x.toFixed(1)}</td><td>${e.pose.y.toFixed(1)}</td><td>${level}</td></tr>`;
    });
  rosterBody.innerHTML = rows.join('');
}

async function teardown(): Promise<void> {
  const s = session;
  session = null;
  joinButton.disabled = false;
  leaveButton.disabled = true;
  rosterBody.innerHTML = '';
  if (!s) return;
  await s.client.close(); // stops capture, playout, presence and state
  await s.ctx.close();
}

form.addEventListener('submit', (ev) => {
  ev.preventDefault(); // a user gesture, which the AudioContext needs
  join().catch((e) => {
    setStatus(`failed: ${e instanceof Error ? e.message : String(e)}`);
    console.error(e);
  });
});
leaveButton.addEventListener('click', () => {
  teardown().then(() => setStatus('left'));
});
