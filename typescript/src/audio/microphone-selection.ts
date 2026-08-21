/**
 * Automatic microphone selection — avoids Bluetooth microphones.
 *
 * Bluetooth audio devices in HFP/SCO mode (required for mic input) force
 * stereo output to collapse to mono. This module selects the best
 * non-Bluetooth microphone available, using label heuristics only.
 *
 * **This module NEVER opens a device.** Opening a Bluetooth mic — even a
 * brief label-population or sample-rate probe — is itself the HFP trigger,
 * and the headset holds the collapsed state afterwards. (The old client
 * learned this 2026-06-11: the gating flow itself triggered HFP on Safari;
 * this port re-learned it 2026-08-06 and dropped the priming open and the
 * unknown-device probe.) Without mic permission there are no labels to
 * classify, so selection reports `permissionPending` and the caller opens
 * its production stream on the system default — the one unavoidable open
 * of a true first use — then re-runs selection once permission exists and
 * switches away if the default turned out Bluetooth.
 */

// ── Bluetooth detection keywords ────────────────────────────────────────────

/** Keywords that strongly indicate a Bluetooth device (case-insensitive). */
const BLUETOOTH_KEYWORDS = [
  'bluetooth',
  'bt ',
  'bt-',
  // HFP/SCO profile indicators (sometimes exposed in device labels)
  'hands-free',
  'handsfree',
  'hfp',
  'sco',
  'a2dp',
];

/** Known Bluetooth product/brand names (case-insensitive). */
const BLUETOOTH_BRANDS = [
  'airpods',
  'beats ',
  'beats+',
  'beatsx',
  'powerbeats',
  'jabra',
  'galaxy buds',
  'buds pro',
  'buds live',
  'buds2',
  'buds fe',
  'sony wh-',
  'sony wf-',
  'bose qc',
  'bose quietcomfort',
  'bose noise cancelling',
  'bose soundsport',
  'bose sport',
  'jbl tune',
  'jbl live',
  'jbl reflect',
  'jbl endurance',
  'sennheiser momentum',
  'sennheiser cx',
  'marshall major',
  'marshall minor',
  'marshall motif',
  'pixel buds',
  'nothing ear',
  'huawei freebuds',
  'oppo enco',
  'oneplus buds',
  'anker soundcore',
  'soundcore liberty',
  'skullcandy',
  'tozo',
  'jlab',
];

/** Keywords indicating a USB microphone. */
const USB_KEYWORDS = [
  'usb',
  // Well-known USB mic brands
  'blue yeti',
  'blue snowball',
  'rode nt-usb',
  'rode podcaster',
  'at2020',
  'at2005',
  'samson',
  'focusrite',
  'scarlett',
  'behringer',
  'presonus',
  'elgato wave',
  'hyperx quadcast',
  'razer seiren',
  'fifine',
  'maono',
  'audio-technica',
  'shure mv',
];

/** Keywords indicating a built-in microphone. */
const BUILTIN_KEYWORDS = [
  'built-in',
  'builtin',
  'internal',
  'macbook',
  'imac',
  'integrated',
  'laptop',
  'webcam',
  'facetime',
];

// ── Classification ──────────────────────────────────────────────────────────

export type MicrophoneType = 'bluetooth' | 'usb' | 'builtin' | 'unknown';

export interface ClassifiedMicrophone {
  deviceId: string;
  label: string;
  type: MicrophoneType;
}

export interface MicrophoneSelectionResult {
  /** The selected device ID (or undefined to use system default). */
  deviceId: string | undefined;
  /** The label of the selected device. */
  label: string;
  /** Classification of the selected device. */
  type: MicrophoneType;
  /** All devices that were evaluated. */
  allDevices: ClassifiedMicrophone[];
  /** Whether the default mic was Bluetooth and we switched away from it. */
  switchedFromBluetooth: boolean;
  /**
   * True when mic permission has not been granted yet: labels are
   * unreadable and no device was (or ever will be, by this module)
   * opened to read them. The caller should open its production stream
   * on the default device, then re-run selection — permission now
   * exists — and switch away if the default proved Bluetooth.
   */
  permissionPending: boolean;
}

/**
 * Classify a microphone based on its label.
 */
export function classifyByLabel(label: string): MicrophoneType {
  const lower = label.toLowerCase();

  // Check Bluetooth first — a device could match both BT and USB keywords
  for (const keyword of BLUETOOTH_KEYWORDS) {
    if (lower.includes(keyword)) return 'bluetooth';
  }
  for (const brand of BLUETOOTH_BRANDS) {
    if (lower.includes(brand)) return 'bluetooth';
  }

  for (const keyword of USB_KEYWORDS) {
    if (lower.includes(keyword)) return 'usb';
  }

  for (const keyword of BUILTIN_KEYWORDS) {
    if (lower.includes(keyword)) return 'builtin';
  }

  return 'unknown';
}

// ── Ranking ─────────────────────────────────────────────────────────────────

/** Priority order: lower = better. */
const TYPE_PRIORITY: Record<MicrophoneType, number> = {
  usb: 0,
  builtin: 1,
  unknown: 2,
  bluetooth: 3,
};

function compareMicrophones(a: ClassifiedMicrophone, b: ClassifiedMicrophone): number {
  return TYPE_PRIORITY[a.type] - TYPE_PRIORITY[b.type];
}

// ── Permission state ─────────────────────────────────────────────────────────

/**
 * Whether mic permission is ALREADY granted. When it is, enumerateDevices()
 * returns full labels without opening any microphone — critical for Bluetooth
 * headsets, where merely opening the mic (even our brief label-population
 * probe) flips the headset into HFP and collapses its output to mono, a state
 * macOS/the device can then hold on to. Returns false when the Permissions API
 * is unavailable (the caller falls back to the one-time getUserMedia probe).
 */
export async function micPermissionGranted(): Promise<boolean> {
  try {
    const permissions = (navigator as Navigator & {
      permissions?: { query(desc: { name: string }): Promise<{ state: string }> };
    }).permissions;
    if (!permissions?.query) return false;
    const status = await permissions.query({ name: 'microphone' });
    return status.state === 'granted';
  } catch {
    return false;
  }
}

// ── Main selection function ─────────────────────────────────────────────────

/**
 * Select the best non-Bluetooth microphone WITHOUT opening any device.
 *
 * Flow:
 * 1. If permission is not yet granted: return `permissionPending` — labels
 *    are unreadable and this module never opens a device to read them (see
 *    module doc). The caller opens its production stream on the default and
 *    re-runs selection afterwards.
 * 2. Enumerate all audio input devices and classify by label.
 * 3. Return the highest-ranked non-Bluetooth device, or fall back to default.
 *
 * @param debug - Enable console logging of the selection process.
 */
export async function selectBestMicrophone(
  debug: boolean = false,
): Promise<MicrophoneSelectionResult> {
  const log = debug
    ? (...args: unknown[]) => console.log('[MicSelection]', ...args)
    : () => {};

  if (!(await micPermissionGranted())) {
    log('Mic permission not granted — no labels readable; deferring to the system default');
    return {
      deviceId: undefined,
      label: '(permission pending)',
      type: 'unknown',
      allDevices: [],
      switchedFromBluetooth: false,
      permissionPending: true,
    };
  }

  // Enumerate and classify by label (no opens: permission is granted, so
  // labels are readable).
  const allDevices = await navigator.mediaDevices.enumerateDevices();
  const mics: ClassifiedMicrophone[] = allDevices
    .filter((d) => d.kind === 'audioinput')
    .map((d) => ({
      deviceId: d.deviceId,
      label: d.label || '(unlabelled)',
      type: classifyByLabel(d.label || ''),
    }));

  log('Enumerated microphones:', mics.map((m) => `${m.label} [${m.type}]`));

  if (mics.length === 0) {
    log('No microphones found, using system default');
    return {
      deviceId: undefined,
      label: '(none)',
      type: 'unknown',
      allDevices: [],
      switchedFromBluetooth: false,
      permissionPending: false,
    };
  }

  // Rank and select. The first entry mirrors the default device (Chrome
  // synthesises a 'default' entry; elsewhere the first listed is a proxy).
  const defaultMic = mics[0]!;
  const defaultIsBluetooth = defaultMic.type === 'bluetooth';

  const ranked = [...mics].sort(compareMicrophones);
  const best = ranked[0]!;

  log('Ranked microphones:', ranked.map((m) => `${m.label} [${m.type}]`));
  log(`Selected: ${best.label} [${best.type}]`);

  if (defaultIsBluetooth && best.type !== 'bluetooth') {
    log(`Switched away from Bluetooth default: ${defaultMic.label}`);
  }

  return {
    deviceId: best.deviceId === 'default' ? undefined : best.deviceId,
    label: best.label,
    type: best.type,
    allDevices: mics,
    switchedFromBluetooth: defaultIsBluetooth && best.type !== 'bluetooth',
    permissionPending: false,
  };
}
