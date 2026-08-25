/**
 * The LASA base profile (`lasa-base-profile.md`) as a typed client API:
 * the `lasa.` key vocabulary, the §1 value codec, and the operations an
 * application performs on it — personal mutes and solos, moderation,
 * channel policy, rendering parameters — plus a typed read view over a
 * {@link Store}.
 *
 * Everything here reduces to state ops (`lasa-core.md` §6). There is no
 * separate command channel: a "command" is a set or clear on a
 * reserved key, ordered and echoed by the server like any other write.
 * The helpers only build keys and encode values; permission is the
 * server's, and a write the caller's roles do not allow is silently
 * dropped (§6.2), never surfaced here.
 *
 * The vocabulary separates two concerns. **Channels** own hearing —
 * who is audible to whom, muted, at what gain and under what distance
 * law — and everything about audibility is expressed through channel
 * membership and channel policy. **Roles** own permission only — who
 * may write which keys. There is deliberately no role-wide mute or
 * gain: to silence or rebalance a group, put it in a channel.
 *
 * Key builders and the codec mirror Go `profile/base` (keys.go,
 * consume.go); a change to the vocabulary lands there first.
 */

import { MalformedError } from './wire.js';
import type { UpstreamClearOp, UpstreamSetOp, UpstreamStateMessage } from './statewire.js';
import type { Store } from './store.js';

// ---------------------------------------------------------------------------
// Values (profile §1)

/** A base-profile value: Boolean, String or Number (`lasa-base-profile.md` §1). */
export type BaseValue = boolean | string | number;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

/**
 * Encodes a §1 value per JSON conventions, byte-for-byte as the Go
 * profile writes them: `true`/`false`, a JSON string, or a shortest
 * round-trip number.
 */
export function encodeValue(v: BaseValue): Uint8Array {
  if (typeof v === 'number' && !Number.isFinite(v)) {
    throw new MalformedError(`base value must be finite, got ${v}`);
  }
  return textEncoder.encode(JSON.stringify(v));
}

/**
 * Decodes a §1 value. Returns undefined for absent bytes, and for bytes
 * that are not one of the three types — the profile says an invalid
 * value is stored and delivered but ignored by every consumer (§2), so
 * a reader treats it as absent.
 */
export function decodeValue(bytes: Uint8Array | undefined): BaseValue | undefined {
  if (!bytes) return undefined;
  let parsed: unknown;
  try {
    parsed = JSON.parse(textDecoder.decode(bytes));
  } catch {
    return undefined;
  }
  switch (typeof parsed) {
    case 'boolean':
    case 'string':
      return parsed;
    case 'number':
      return Number.isFinite(parsed) ? parsed : undefined;
    default:
      return undefined;
  }
}

// ---------------------------------------------------------------------------
// Identifiers and keys (core §2, profile §6)

const IDENTIFIER_RE = /^[a-z0-9-]{1,128}$/;
const ATTR_PATH_RE = /^[a-z0-9-]{1,128}(\.[a-z0-9-]{1,128})*$/;

/** Reports whether `s` matches the identifier grammar `[a-z0-9-]{1,128}` (`lasa-core.md` §2). */
export function isIdentifier(s: string): boolean {
  return IDENTIFIER_RE.test(s);
}

function ident(s: string, what: string): string {
  if (!IDENTIFIER_RE.test(s)) throw new MalformedError(`invalid ${what} ${JSON.stringify(s)}: must match [a-z0-9-]{1,128}`);
  return s;
}

function attrPath(s: string): string {
  if (!ATTR_PATH_RE.test(s)) {
    throw new MalformedError(`invalid attr path ${JSON.stringify(s)}: dotted identifiers only`);
  }
  return s;
}

/** The reserved base-profile key prefix. */
export const BASE_PREFIX = 'lasa.';

/**
 * Key builders for the base-profile vocabulary (`lasa-base-profile.md`
 * §6). Each validates its identifier arguments and throws
 * {@link MalformedError} on a violation, so a bad id fails at the call
 * rather than as a silently-dropped write.
 */
export const keys = {
  /** `lasa.entity.{id}` — the existence marker, valued with the owning client id. */
  entityMarker: (id: string) => `lasa.entity.${ident(id, 'entity id')}`,
  /** `lasa.entity.{id}.{suffix}`. The suffix is not validated; prefer a specific builder. */
  entity: (id: string, suffix: string) => `lasa.entity.${ident(id, 'entity id')}.${suffix}`,
  name: (id: string) => keys.entity(id, 'name'),
  attr: (id: string, path: string) => keys.entity(id, `attrs.${attrPath(path)}`),
  signedAttr: (id: string, path: string) => keys.entity(id, `signed-attrs.${attrPath(path)}`),
  heardIn: (id: string, channel: string) => keys.entity(id, `heard-in.${ident(channel, 'channel')}`),
  hears: (id: string, channel: string) => keys.entity(id, `hears.${ident(channel, 'channel')}`),
  priority: (id: string) => keys.entity(id, 'priority'),
  renderGain: (id: string) => keys.entity(id, 'render.gain'),
  renderAttenuation: (id: string) => keys.entity(id, 'render.attenuation'),
  renderSize: (id: string) => keys.entity(id, 'render.size'),
  renderDirectivity: (id: string) => keys.entity(id, 'render.directivity'),
  frame: (id: string) => keys.entity(id, 'frame'),
  dof: (id: string) => keys.entity(id, 'dof'),
  personalMute: (owner: string, other: string) => keys.entity(owner, `mute.${ident(other, 'entity id')}`),
  personalSolo: (owner: string, other: string) => keys.entity(owner, `solo.${ident(other, 'entity id')}`),

  spaceEntityMuted: (id: string) => `lasa.space.entity.muted.${ident(id, 'entity id')}`,
  clientBlocked: (id: string) => `lasa.space.client.blocked.${ident(id, 'client id')}`,
  roleBlocked: (role: string) => `lasa.space.role.blocked.${ident(role, 'role')}`,
  channelMuted: (ch: string) => `lasa.space.channel.muted.${ident(ch, 'channel')}`,
  channelGain: (ch: string) => `lasa.space.channel.gain.${ident(ch, 'channel')}`,
  channelAttenuation: (ch: string) => `lasa.space.channel.attenuation.${ident(ch, 'channel')}`,
} as const;

// ---------------------------------------------------------------------------
// Op construction

function set(key: string, value: BaseValue): UpstreamSetOp {
  return { kind: 'set', key, value: encodeValue(value) };
}
function clear(key: string): UpstreamClearOp {
  return { kind: 'clear', key };
}
/**
 * One op stays a bare op; several become a group — a hint that they be
 * applied together (`lasa-core.md` §6: no consistency guarantee, and an
 * over-cap group is dropped whole).
 */
function oneOrGroup(ops: (UpstreamSetOp | UpstreamClearOp)[]): UpstreamStateMessage[] {
  if (ops.length === 0) return [];
  if (ops.length === 1) return [ops[0]!];
  return [{ kind: 'group', ops }];
}

function range(name: string, v: number, lo: number, hi: number): number {
  if (!Number.isFinite(v) || v < lo || v > hi) {
    throw new MalformedError(`${name} must be in ${lo}–${hi}, got ${v}`);
  }
  return v;
}

/**
 * Converts a block expiry to the profile's wire value: 0 for forever,
 * otherwise unix seconds. A Date in the past is still sent as given —
 * the server's clock decides expiry, not ours.
 */
function blockedValue(until: Date | undefined): number {
  if (until === undefined) return 0;
  const secs = Math.floor(until.getTime() / 1000);
  if (!Number.isFinite(secs) || secs <= 0) throw new MalformedError(`invalid block expiry ${until}`);
  return secs;
}

/** Anything that can publish upstream state ops — a {@link LasaClient}, or a test double. */
export interface StateWriter {
  writeState(...msgs: UpstreamStateMessage[]): Promise<void>;
}

// ---------------------------------------------------------------------------
// Owner-scope operations (profile §6: attrs, mute.*, solo.*)

/** Render parameters (profile §6 `render.*`). Undefined fields are left untouched. */
export interface RenderParams {
  /** Amplitude multiplier 0.0–3.0; absent = 1.0. */
  gain?: number;
  /** Distance-attenuation exponent 0.0–3.0; absent = 2.0 (inverse-square). */
  attenuation?: number;
  /** Source size in metres, ≥ 0; absent = point source. */
  size?: number;
  /** 0.0 omnidirectional – 1.0 maximally directional. */
  directivity?: number;
}

/**
 * The operations an entity's owner may perform on it: personal mutes
 * and solos, and runtime attrs. Reached as methods on
 * {@link EntityPublisher} (`const me = await client.entity(id)`), or
 * constructed directly when the publisher is not needed.
 *
 * Mute is bidirectional (§6): muting `other` silences the pair both
 * ways. Solo silences everyone but the solo'd set for this listener
 * only, and wins over mutes. Both are connection-scoped and reset on
 * reconnect — a client rewrites what it knows.
 */
export class EntityControls {
  constructor(
    private readonly writer: StateWriter,
    /** The owned entity these controls act for. */
    readonly entityId: string
  ) {
    ident(entityId, 'entity id');
  }

  /** Silences the pair (this entity, `other`) in both directions. */
  mute(other: string): Promise<void> {
    return this.writer.writeState(set(keys.personalMute(this.entityId, other), true));
  }
  unmute(other: string): Promise<void> {
    return this.writer.writeState(clear(keys.personalMute(this.entityId, other)));
  }
  /** Hear `other` and the other solo'd entities only, overriding mutes. */
  solo(other: string): Promise<void> {
    return this.writer.writeState(set(keys.personalSolo(this.entityId, other), true));
  }
  unsolo(other: string): Promise<void> {
    return this.writer.writeState(clear(keys.personalSolo(this.entityId, other)));
  }

  /** Sets one runtime attribute, `path` dotted per §1 (`colour`, `avatar.hat`). */
  setAttr(path: string, value: BaseValue): Promise<void> {
    return this.writer.writeState(set(keys.attr(this.entityId, path), value));
  }
  clearAttr(path: string): Promise<void> {
    return this.writer.writeState(clear(keys.attr(this.entityId, path)));
  }
  /** Sets several attributes as one group (flat: one entry per leaf path). */
  setAttrs(attrs: Record<string, BaseValue>): Promise<void> {
    return this.writer.writeState(
      ...oneOrGroup(Object.entries(attrs).map(([p, v]) => set(keys.attr(this.entityId, p), v)))
    );
  }
}

// ---------------------------------------------------------------------------
// Space-scope operations (profile §4 channels, §5 moderation, §6 render)

/**
 * Space-wide operations, reached as `client.space`. Each is gated by
 * role on the server (`moderator` for entity mutes and client blocks,
 * `producer` for channels, rendering and role blocks — profile §2) and
 * silently dropped when the caller lacks it.
 *
 * Grouped by concern: **hearing** is channel membership and channel
 * policy; **moderation** is blocks, which act on the connection; the
 * space entity mute is the one moderation act on hearing.
 */
export class SpaceControls {
  constructor(private readonly writer: StateWriter) {}

  // --- moderation (§5) ---

  /** Silences the entity's source in every render. Rendering only; the connection stays. */
  muteEntity(entityId: string): Promise<void> {
    return this.writer.writeState(set(keys.spaceEntityMuted(entityId), true));
  }
  unmuteEntity(entityId: string): Promise<void> {
    return this.writer.writeState(clear(keys.spaceEntityMuted(entityId)));
  }

  /**
   * Terminates the client's live connection and refuses readmission
   * until `until` (forever when omitted). Admin is unblockable.
   */
  blockClient(clientId: string, until?: Date): Promise<void> {
    return this.writer.writeState(set(keys.clientBlocked(clientId), blockedValue(until)));
  }
  unblockClient(clientId: string): Promise<void> {
    return this.writer.writeState(clear(keys.clientBlocked(clientId)));
  }
  /** A block with a short expiry — "a kick is a block with a short expiry" (§5). */
  kickClient(clientId: string, forMs: number): Promise<void> {
    if (!(forMs > 0)) throw new MalformedError(`kick duration must be positive, got ${forMs}`);
    return this.blockClient(clientId, new Date(Date.now() + forMs));
  }

  /** Blocks every connection bearing the role (except admin). */
  blockRole(role: string, until?: Date): Promise<void> {
    return this.writer.writeState(set(keys.roleBlocked(role), blockedValue(until)));
  }
  unblockRole(role: string): Promise<void> {
    return this.writer.writeState(clear(keys.roleBlocked(role)));
  }

  // --- hearing: channel policy (§4) ---

  /** A muted channel contributes nothing to audibility. */
  muteChannel(channel: string): Promise<void> {
    return this.writer.writeState(set(keys.channelMuted(channel), true));
  }
  unmuteChannel(channel: string): Promise<void> {
    return this.writer.writeState(clear(keys.channelMuted(channel)));
  }
  /** Channel gain 0–3.0 (amplitude); the highest across a pair's shared channels wins. `undefined` clears. */
  setChannelGain(channel: string, gain: number | undefined): Promise<void> {
    const key = keys.channelGain(channel);
    return this.writer.writeState(gain === undefined ? clear(key) : set(key, range('channel gain', gain, 0, 3)));
  }
  /** Channel attenuation 0–3.0; the lowest set value across a pair's shared channels overrides the source's. `undefined` clears. */
  setChannelAttenuation(channel: string, attenuation: number | undefined): Promise<void> {
    const key = keys.channelAttenuation(channel);
    return this.writer.writeState(
      attenuation === undefined ? clear(key) : set(key, range('channel attenuation', attenuation, 0, 3))
    );
  }

  // --- hearing: entity membership and rendering (§4, §6) ---

  /** Adds the entity's source to a channel (it is heard there). */
  addHeardIn(entityId: string, channel: string): Promise<void> {
    return this.writer.writeState(set(keys.heardIn(entityId, channel), true));
  }
  removeHeardIn(entityId: string, channel: string): Promise<void> {
    return this.writer.writeState(clear(keys.heardIn(entityId, channel)));
  }
  /** Adds the entity's sink to a channel (it hears there). */
  addHears(entityId: string, channel: string): Promise<void> {
    return this.writer.writeState(set(keys.hears(entityId, channel), true));
  }
  removeHears(entityId: string, channel: string): Promise<void> {
    return this.writer.writeState(clear(keys.hears(entityId, channel)));
  }
  /**
   * Replaces the entity's memberships as one group: `current` is what
   * the store holds now (see {@link BaseView.entity}), `next` the wanted
   * sets. Omitted sides are left alone. Emptying `heardIn` leaves the
   * entity inaudible (§6) — a moderation act, so it is allowed.
   */
  setChannels(
    entityId: string,
    current: { heardIn: readonly string[]; hears: readonly string[] },
    next: { heardIn?: readonly string[]; hears?: readonly string[] }
  ): Promise<void> {
    const ops: (UpstreamSetOp | UpstreamClearOp)[] = [];
    const diff = (have: readonly string[], want: readonly string[] | undefined, key: (ch: string) => string) => {
      if (!want) return;
      const wantSet = new Set(want);
      for (const ch of have) if (!wantSet.has(ch)) ops.push(clear(key(ch)));
      for (const ch of want) if (!have.includes(ch)) ops.push(set(key(ch), true));
    };
    diff(current.heardIn, next.heardIn, (ch) => keys.heardIn(entityId, ch));
    diff(current.hears, next.hears, (ch) => keys.hears(entityId, ch));
    return this.writer.writeState(...oneOrGroup(ops));
  }

  /** Sets the given render parameters as one group; `undefined` fields are untouched, `null` clears to the default. */
  setRender(entityId: string, params: { [K in keyof RenderParams]?: number | null }): Promise<void> {
    const ops: (UpstreamSetOp | UpstreamClearOp)[] = [];
    const field = (v: number | null | undefined, key: string, name: string, lo: number, hi: number) => {
      if (v === undefined) return;
      ops.push(v === null ? clear(key) : set(key, range(name, v, lo, hi)));
    };
    field(params.gain, keys.renderGain(entityId), 'gain', 0, 3);
    field(params.attenuation, keys.renderAttenuation(entityId), 'attenuation', 0, 3);
    field(params.size, keys.renderSize(entityId), 'size', 0, Infinity);
    field(params.directivity, keys.renderDirectivity(entityId), 'directivity', 0, 1);
    return this.writer.writeState(...oneOrGroup(ops));
  }

  /** Marks the entity's source as important to render well — sparingly (§6). */
  setPriority(entityId: string, priority: boolean): Promise<void> {
    const key = keys.priority(entityId);
    return this.writer.writeState(priority ? set(key, true) : clear(key));
  }

  /** Degrees of freedom the space enforces at ingest: 6 free, 3 turn in place, 0 fixed (`moderator`). */
  setDof(entityId: string, dof: 0 | 3 | 6): Promise<void> {
    if (dof !== 0 && dof !== 3 && dof !== 6) throw new MalformedError(`dof must be 0, 3 or 6, got ${dof}`);
    return this.writer.writeState(dof === 6 ? clear(keys.dof(entityId)) : set(keys.dof(entityId), dof));
  }
}

// ---------------------------------------------------------------------------
// Read view (profile §6, over a Store subscribed to `lasa.`)

/** One live entity as the store describes it. */
export interface EntityView {
  id: string;
  /** The owning client id (the existence marker's value). */
  owner: string;
  name: string;
  /** Runtime attributes, flat by dotted path. */
  attrs: Record<string, BaseValue>;
  /** Attributes from inside the signed ticket, immutable. */
  signedAttrs: Record<string, BaseValue>;
  /** Channels in which the source is heard. */
  heardIn: string[];
  /** Channels the sink listens to. */
  hears: string[];
  priority: boolean;
  /** Set render parameters only — absent fields take the §6 defaults. */
  render: RenderParams;
  frame: 'world' | 'head';
  dof: 0 | 3 | 6;
  /** Entities this entity has personally muted / solo'd (readable by every role, §2: a muted user can see it). */
  mutes: string[];
  solos: string[];
}

/** A block entry: `until` is null for forever, else the expiry. */
export interface BlockView {
  id: string;
  until: Date | null;
}

const ENTITY_RE = /^lasa\.entity\.([a-z0-9-]{1,128})(?:\.(.+))?$/;

/**
 * A typed read view over a {@link Store} whose prefixes include
 * `lasa.`. Every call scans the store, which is the right shape for a
 * few hundred keys; an application with more keeps its own index from
 * `store` iteration. Read after `await sync.settled()`.
 *
 * Keys under an entity whose existence marker is absent are ignored,
 * as the profile requires (§6) — a departing entity's subtree can be
 * mid-sweep.
 */
export class BaseView {
  constructor(private readonly store: Store) {}

  /** Every live entity (marker present), in key order. */
  entities(): EntityView[] {
    const views = new Map<string, EntityView>();
    // Pass 1: markers, so we know which subtrees are live.
    for (const [key, kv] of this.store) {
      const m = ENTITY_RE.exec(key);
      if (!m || m[2] !== undefined) continue;
      const owner = decodeValue(kv.value);
      if (typeof owner !== 'string') continue;
      views.set(m[1]!, emptyView(m[1]!, owner));
    }
    // Pass 2: everything under a live marker.
    for (const [key, kv] of this.store) {
      const m = ENTITY_RE.exec(key);
      if (!m || m[2] === undefined) continue;
      const v = views.get(m[1]!);
      if (v) applyEntityKey(v, m[2], decodeValue(kv.value));
    }
    return [...views.keys()].sort().map((id) => views.get(id)!);
  }

  /** One entity, or undefined if its marker is absent. */
  entity(id: string): EntityView | undefined {
    const owner = decodeValue(this.store.get(keys.entityMarker(id)));
    if (typeof owner !== 'string') return undefined;
    const v = emptyView(id, owner);
    const prefix = `lasa.entity.${id}.`;
    for (const [key, kv] of this.store) {
      if (key.startsWith(prefix)) applyEntityKey(v, key.slice(prefix.length), decodeValue(kv.value));
    }
    return v;
  }

  /** Whether the space has muted the entity's source (§5). */
  isEntityMuted(entityId: string): boolean {
    return decodeValue(this.store.get(keys.spaceEntityMuted(entityId))) === true;
  }

  /** The client's block, or undefined when not blocked. Expiry is not evaluated here — the server's clock decides. */
  clientBlock(clientId: string): BlockView | undefined {
    return blockView(clientId, decodeValue(this.store.get(keys.clientBlocked(clientId))));
  }
  roleBlock(role: string): BlockView | undefined {
    return blockView(role, decodeValue(this.store.get(keys.roleBlocked(role))));
  }
  /** Every blocked client. */
  blockedClients(): BlockView[] {
    return this.collectBlocks('lasa.space.client.blocked.');
  }
  blockedRoles(): BlockView[] {
    return this.collectBlocks('lasa.space.role.blocked.');
  }

  /** Channel policy: muted flag and any set gain/attenuation override. */
  channel(channel: string): { muted: boolean; gain?: number; attenuation?: number } {
    const gain = decodeValue(this.store.get(keys.channelGain(channel)));
    const att = decodeValue(this.store.get(keys.channelAttenuation(channel)));
    return {
      muted: decodeValue(this.store.get(keys.channelMuted(channel))) === true,
      ...(typeof gain === 'number' ? { gain } : {}),
      ...(typeof att === 'number' ? { attenuation: att } : {}),
    };
  }

  /** Every channel named by a live entity's membership or by channel policy, sorted. */
  channels(): string[] {
    const out = new Set<string>();
    for (const e of this.entities()) {
      for (const ch of e.heardIn) out.add(ch);
      for (const ch of e.hears) out.add(ch);
    }
    for (const [key] of this.store) {
      const m = /^lasa\.space\.channel\.(?:muted|gain|attenuation)\.([a-z0-9-]{1,128})$/.exec(key);
      if (m) out.add(m[1]!);
    }
    return [...out].sort();
  }

  private collectBlocks(prefix: string): BlockView[] {
    const out: BlockView[] = [];
    for (const [key, kv] of this.store) {
      if (!key.startsWith(prefix)) continue;
      const id = key.slice(prefix.length);
      if (!isIdentifier(id)) continue;
      const b = blockView(id, decodeValue(kv.value));
      if (b) out.push(b);
    }
    return out.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  }
}

/** Builds a {@link BaseView} over a store. */
export function baseView(store: Store): BaseView {
  return new BaseView(store);
}

function emptyView(id: string, owner: string): EntityView {
  return {
    id,
    owner,
    name: '',
    attrs: {},
    signedAttrs: {},
    heardIn: [],
    hears: [],
    priority: false,
    render: {},
    frame: 'world',
    dof: 6,
    mutes: [],
    solos: [],
  };
}

function blockView(id: string, v: BaseValue | undefined): BlockView | undefined {
  if (typeof v !== 'number' || v < 0) return undefined;
  return { id, until: v === 0 ? null : new Date(v * 1000) };
}

function applyEntityKey(v: EntityView, suffix: string, value: BaseValue | undefined): void {
  if (value === undefined) return;
  const dot = suffix.indexOf('.');
  const head = dot < 0 ? suffix : suffix.slice(0, dot);
  const rest = dot < 0 ? '' : suffix.slice(dot + 1);
  switch (head) {
    case 'name':
      if (typeof value === 'string') v.name = value;
      break;
    case 'attrs':
      if (rest) v.attrs[rest] = value;
      break;
    case 'signed-attrs':
      if (rest) v.signedAttrs[rest] = value;
      break;
    case 'heard-in':
      if (rest && value === true) v.heardIn.push(rest);
      break;
    case 'hears':
      if (rest && value === true) v.hears.push(rest);
      break;
    case 'priority':
      v.priority = value === true;
      break;
    case 'render': {
      if (typeof value !== 'number') break;
      if (rest === 'gain') v.render.gain = value;
      else if (rest === 'attenuation') v.render.attenuation = value;
      else if (rest === 'size') v.render.size = value;
      else if (rest === 'directivity') v.render.directivity = value;
      break;
    }
    case 'frame':
      if (value === 'head' || value === 'world') v.frame = value;
      break;
    case 'dof':
      if (value === 0 || value === 3 || value === 6) v.dof = value;
      break;
    case 'mute':
      if (rest && value === true) v.mutes.push(rest);
      break;
    case 'solo':
      if (rest && value === true) v.solos.push(rest);
      break;
  }
}
