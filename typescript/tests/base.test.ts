/**
 * Base-profile helper tests: key builders and the value codec against
 * the literals Go `profile/base` writes and reads (keys.go, base.go
 * AdmissionGroup, consume.go parseFlag/parseNumber), the ops each
 * control emits, and the read view over a populated Store.
 */
import { describe, expect, it } from 'vitest';
import {
  BaseView,
  EntityControls,
  SpaceControls,
  decodeValue,
  encodeValue,
  isIdentifier,
  keys,
  type StateWriter,
} from '../src/lasa/base';
import { MalformedError } from '../src/lasa/wire';
import { Store } from '../src/lasa/store';
import type { StateMessage, UpstreamStateMessage } from '../src/lasa/statewire';

const enc = new TextEncoder();
const dec = new TextDecoder();
const text = (b: Uint8Array) => dec.decode(b);

/** Captures writes; `last` is the most recent call's messages. */
class FakeWriter implements StateWriter {
  calls: UpstreamStateMessage[][] = [];
  async writeState(...msgs: UpstreamStateMessage[]): Promise<void> {
    this.calls.push(msgs);
  }
  get last(): UpstreamStateMessage[] {
    return this.calls[this.calls.length - 1]!;
  }
}

/** Renders ops as `key=value` / `-key` strings for compact assertions. */
function flat(msgs: UpstreamStateMessage[]): string[] {
  const one = (m: UpstreamStateMessage): string[] => {
    switch (m.kind) {
      case 'set':
        return [`${m.key}=${text(m.value)}`];
      case 'clear':
        return [`-${m.key}`];
      case 'group':
        return ['group(', ...m.ops.flatMap(one), ')'];
      default:
        return [m.kind];
    }
  };
  return msgs.flatMap(one);
}

describe('value codec (§1, parity with Go literals)', () => {
  it('encodes the three types as Go writes them', () => {
    expect(text(encodeValue(true))).toBe('true');
    expect(text(encodeValue(false))).toBe('false');
    expect(text(encodeValue('head'))).toBe('"head"');
    expect(text(encodeValue('c-1'))).toBe('"c-1"');
    expect(text(encodeValue(1.5))).toBe('1.5');
    expect(text(encodeValue(6))).toBe('6');
    expect(text(encodeValue(0))).toBe('0');
    expect(text(encodeValue(1700000000))).toBe('1700000000');
  });

  it('rejects non-finite numbers', () => {
    expect(() => encodeValue(NaN)).toThrow(MalformedError);
    expect(() => encodeValue(Infinity)).toThrow(MalformedError);
  });

  it('decodes Go-written bytes, incl. AppendFloat "g" forms', () => {
    expect(decodeValue(enc.encode('true'))).toBe(true);
    expect(decodeValue(enc.encode('"alice"'))).toBe('alice');
    expect(decodeValue(enc.encode('2.3'))).toBe(2.3);
    expect(decodeValue(enc.encode('1e+06'))).toBe(1_000_000);
    expect(decodeValue(enc.encode('6'))).toBe(6);
  });

  it('treats absent and invalid values as absent (§2: ignored by consumers)', () => {
    expect(decodeValue(undefined)).toBeUndefined();
    expect(decodeValue(enc.encode(''))).toBeUndefined();
    expect(decodeValue(enc.encode('not json'))).toBeUndefined();
    expect(decodeValue(enc.encode('[1,2]'))).toBeUndefined();
    expect(decodeValue(enc.encode('{"a":1}'))).toBeUndefined();
    expect(decodeValue(enc.encode('null'))).toBeUndefined();
  });
});

describe('key builders (§6, parity with Go keys.go)', () => {
  it('builds every vocabulary key', () => {
    expect(keys.entityMarker('e1')).toBe('lasa.entity.e1');
    expect(keys.name('e1')).toBe('lasa.entity.e1.name');
    expect(keys.attr('e1', 'colour')).toBe('lasa.entity.e1.attrs.colour');
    expect(keys.attr('e1', 'avatar.hat')).toBe('lasa.entity.e1.attrs.avatar.hat');
    expect(keys.signedAttr('e1', 'tier')).toBe('lasa.entity.e1.signed-attrs.tier');
    expect(keys.heardIn('e1', 'main')).toBe('lasa.entity.e1.heard-in.main');
    expect(keys.hears('e1', 'stage')).toBe('lasa.entity.e1.hears.stage');
    expect(keys.priority('e1')).toBe('lasa.entity.e1.priority');
    expect(keys.renderGain('e1')).toBe('lasa.entity.e1.render.gain');
    expect(keys.renderAttenuation('e1')).toBe('lasa.entity.e1.render.attenuation');
    expect(keys.renderSize('e1')).toBe('lasa.entity.e1.render.size');
    expect(keys.renderDirectivity('e1')).toBe('lasa.entity.e1.render.directivity');
    expect(keys.frame('e1')).toBe('lasa.entity.e1.frame');
    expect(keys.dof('e1')).toBe('lasa.entity.e1.dof');
    expect(keys.personalMute('e1', 'e2')).toBe('lasa.entity.e1.mute.e2');
    expect(keys.personalSolo('e1', 'e2')).toBe('lasa.entity.e1.solo.e2');
    expect(keys.spaceEntityMuted('e1')).toBe('lasa.space.entity.muted.e1');
    expect(keys.clientBlocked('c1')).toBe('lasa.space.client.blocked.c1');
    expect(keys.roleBlocked('noisy')).toBe('lasa.space.role.blocked.noisy');
    expect(keys.channelMuted('stage')).toBe('lasa.space.channel.muted.stage');
    expect(keys.channelGain('stage')).toBe('lasa.space.channel.gain.stage');
    expect(keys.channelAttenuation('stage')).toBe('lasa.space.channel.attenuation.stage');
  });

  it('accepts a lowercase UUID and rejects grammar violations', () => {
    const uuid = '550e8400-e29b-41d4-a716-446655440000';
    expect(isIdentifier(uuid)).toBe(true);
    expect(keys.entityMarker(uuid)).toBe(`lasa.entity.${uuid}`);
    for (const bad of ['', 'Alice', 'a.b', 'a_b', 'a'.repeat(129), '550E8400-E29B-41D4-A716-446655440000']) {
      expect(isIdentifier(bad)).toBe(false);
      expect(() => keys.entityMarker(bad)).toThrow(MalformedError);
    }
    expect(() => keys.personalMute('e1', 'Other')).toThrow(MalformedError);
    expect(() => keys.heardIn('e1', 'Main')).toThrow(MalformedError);
    // Attr paths are dotted identifiers, never empty segments.
    expect(() => keys.attr('e1', 'a..b')).toThrow(MalformedError);
    expect(() => keys.attr('e1', '.a')).toThrow(MalformedError);
    expect(() => keys.attr('e1', 'Colour')).toThrow(MalformedError);
  });
});

describe('EntityControls (owner scope)', () => {
  it('emits one op per call on the owner-scoped keys', async () => {
    const w = new FakeWriter();
    const me = new EntityControls(w, 'e1');
    await me.mute('e2');
    expect(flat(w.last)).toEqual(['lasa.entity.e1.mute.e2=true']);
    await me.unmute('e2');
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.mute.e2']);
    await me.solo('e3');
    expect(flat(w.last)).toEqual(['lasa.entity.e1.solo.e3=true']);
    await me.unsolo('e3');
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.solo.e3']);
    await me.setAttr('colour', '00aaff');
    expect(flat(w.last)).toEqual(['lasa.entity.e1.attrs.colour="00aaff"']);
    await me.clearAttr('colour');
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.attrs.colour']);
  });

  it('groups several attrs into one write; a single attr stays bare', async () => {
    const w = new FakeWriter();
    const me = new EntityControls(w, 'e1');
    await me.setAttrs({ colour: '00aaff', 'avatar.hat': true, score: 3 });
    expect(flat(w.last)).toEqual([
      'group(',
      'lasa.entity.e1.attrs.colour="00aaff"',
      'lasa.entity.e1.attrs.avatar.hat=true',
      'lasa.entity.e1.attrs.score=3',
      ')',
    ]);
    await me.setAttrs({ colour: 'ff0000' });
    expect(flat(w.last)).toEqual(['lasa.entity.e1.attrs.colour="ff0000"']);
    await me.setAttrs({});
    expect(w.last).toEqual([]);
  });

  it('validates ids at construction and per call', () => {
    const w = new FakeWriter();
    expect(() => new EntityControls(w, 'Me')).toThrow(MalformedError);
    const me = new EntityControls(w, 'e1');
    expect(() => me.mute('Bad')).toThrow(MalformedError);
    expect(w.calls).toEqual([]);
  });
});

describe('SpaceControls (moderation, channels, rendering)', () => {
  it('moderation: entity mute and blocks (0 = forever, else unix seconds)', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    await space.muteEntity('e1');
    expect(flat(w.last)).toEqual(['lasa.space.entity.muted.e1=true']);
    await space.unmuteEntity('e1');
    expect(flat(w.last)).toEqual(['-lasa.space.entity.muted.e1']);

    await space.blockClient('c1');
    expect(flat(w.last)).toEqual(['lasa.space.client.blocked.c1=0']);
    await space.blockClient('c1', new Date(1_700_000_000_500));
    expect(flat(w.last)).toEqual(['lasa.space.client.blocked.c1=1700000000']);
    await space.unblockClient('c1');
    expect(flat(w.last)).toEqual(['-lasa.space.client.blocked.c1']);

    await space.blockRole('noisy', new Date(1_700_000_000_000));
    expect(flat(w.last)).toEqual(['lasa.space.role.blocked.noisy=1700000000']);
    await space.unblockRole('noisy');
    expect(flat(w.last)).toEqual(['-lasa.space.role.blocked.noisy']);
  });

  it('kick is a block expiring forMs from now', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    const before = Math.floor(Date.now() / 1000);
    await space.kickClient('c1', 5 * 60_000);
    const [op] = w.last;
    expect(op!.kind).toBe('set');
    const until = decodeValue((op as { value: Uint8Array }).value) as number;
    expect(until).toBeGreaterThanOrEqual(before + 300);
    expect(until).toBeLessThanOrEqual(before + 301);
    expect(() => space.kickClient('c1', 0)).toThrow(MalformedError);
    expect(() => space.kickClient('c1', -1)).toThrow(MalformedError);
  });

  it('channel policy: mute, gain, attenuation; undefined clears; ranges enforced', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    await space.muteChannel('stage');
    expect(flat(w.last)).toEqual(['lasa.space.channel.muted.stage=true']);
    await space.unmuteChannel('stage');
    expect(flat(w.last)).toEqual(['-lasa.space.channel.muted.stage']);
    await space.setChannelGain('stage', 1.5);
    expect(flat(w.last)).toEqual(['lasa.space.channel.gain.stage=1.5']);
    await space.setChannelGain('stage', undefined);
    expect(flat(w.last)).toEqual(['-lasa.space.channel.gain.stage']);
    await space.setChannelAttenuation('whisper', 3);
    expect(flat(w.last)).toEqual(['lasa.space.channel.attenuation.whisper=3']);
    await space.setChannelAttenuation('whisper', undefined);
    expect(flat(w.last)).toEqual(['-lasa.space.channel.attenuation.whisper']);
    expect(() => space.setChannelGain('stage', 3.1)).toThrow(MalformedError);
    expect(() => space.setChannelAttenuation('stage', -0.1)).toThrow(MalformedError);
  });

  it('membership: single adds/removes, and an atomic diffed replace', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    await space.addHeardIn('e1', 'stage');
    expect(flat(w.last)).toEqual(['lasa.entity.e1.heard-in.stage=true']);
    await space.removeHears('e1', 'main');
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.hears.main']);

    await space.setChannels('e1', { heardIn: ['main'], hears: ['main', 'stage'] }, { heardIn: ['stage'], hears: ['stage'] });
    expect(flat(w.last)).toEqual([
      'group(',
      '-lasa.entity.e1.heard-in.main',
      'lasa.entity.e1.heard-in.stage=true',
      '-lasa.entity.e1.hears.main',
      ')',
    ]);
    // Only the given side changes; identical sets write nothing.
    await space.setChannels('e1', { heardIn: ['main'], hears: ['main'] }, { hears: ['main'] });
    expect(w.last).toEqual([]);
    // Emptying heard-in is permitted (a moderation act, §6).
    await space.setChannels('e1', { heardIn: ['main'], hears: ['main'] }, { heardIn: [] });
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.heard-in.main']);
  });

  it('rendering: partial atomic set, null clears, ranges enforced', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    await space.setRender('e1', { gain: 1.5, attenuation: 2.3 });
    expect(flat(w.last)).toEqual([
      'group(',
      'lasa.entity.e1.render.gain=1.5',
      'lasa.entity.e1.render.attenuation=2.3',
      ')',
    ]);
    await space.setRender('e1', { gain: null });
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.render.gain']);
    await space.setRender('e1', { size: 2, directivity: 0.5 });
    expect(flat(w.last)).toEqual(['group(', 'lasa.entity.e1.render.size=2', 'lasa.entity.e1.render.directivity=0.5', ')']);
    await space.setRender('e1', {});
    expect(w.last).toEqual([]);
    expect(() => space.setRender('e1', { gain: 4 })).toThrow(MalformedError);
    expect(() => space.setRender('e1', { directivity: 1.5 })).toThrow(MalformedError);
    expect(() => space.setRender('e1', { size: -1 })).toThrow(MalformedError);
  });

  it('priority and dof: the default is expressed as clear', async () => {
    const w = new FakeWriter();
    const space = new SpaceControls(w);
    await space.setPriority('e1', true);
    expect(flat(w.last)).toEqual(['lasa.entity.e1.priority=true']);
    await space.setPriority('e1', false);
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.priority']);
    await space.setDof('e1', 3);
    expect(flat(w.last)).toEqual(['lasa.entity.e1.dof=3']);
    await space.setDof('e1', 0);
    expect(flat(w.last)).toEqual(['lasa.entity.e1.dof=0']);
    await space.setDof('e1', 6);
    expect(flat(w.last)).toEqual(['-lasa.entity.e1.dof']);
    expect(() => space.setDof('e1', 4 as 0)).toThrow(MalformedError);
  });
});

describe('BaseView (read side)', () => {
  const set = (key: string, value: string, seq: bigint): StateMessage => ({
    kind: 'set',
    key,
    value: enc.encode(value),
    seq,
  });

  async function populated(): Promise<Store> {
    const s = await Store.create(['lasa.']);
    let seq = 1n;
    const put = (k: string, v: string) => s.applyLive(set(k, v, seq++));
    // e1: a full admission group as the Go server writes it, plus runtime keys.
    await put('lasa.entity.e1', '"c1"');
    await put('lasa.entity.e1.name', '"Alice"');
    await put('lasa.entity.e1.heard-in.main', 'true');
    await put('lasa.entity.e1.heard-in.stage', 'true');
    await put('lasa.entity.e1.hears.main', 'true');
    await put('lasa.entity.e1.priority', 'true');
    await put('lasa.entity.e1.render.gain', '1.5');
    await put('lasa.entity.e1.render.attenuation', '2.3');
    await put('lasa.entity.e1.dof', '3');
    await put('lasa.entity.e1.attrs.colour', '"00aaff"');
    await put('lasa.entity.e1.attrs.avatar.hat', 'true');
    await put('lasa.entity.e1.signed-attrs.tier', '"gold"');
    await put('lasa.entity.e1.mute.e2', 'true');
    await put('lasa.entity.e1.solo.e3', 'true');
    // e2: minimal, head-frame.
    await put('lasa.entity.e2', '"c2"');
    await put('lasa.entity.e2.name', '"Bob"');
    await put('lasa.entity.e2.heard-in.main', 'true');
    await put('lasa.entity.e2.hears.main', 'true');
    await put('lasa.entity.e2.frame', '"head"');
    // e9: keys with NO marker — a departed entity mid-sweep. Must be ignored.
    await put('lasa.entity.e9.name', '"Ghost"');
    await put('lasa.entity.e9.heard-in.main', 'true');
    // Invalid values are ignored by consumers (§2).
    await put('lasa.entity.e2.render.gain', 'loud');
    await put('lasa.entity.e2.dof', '4');
    // Space keys.
    await put('lasa.space.entity.muted.e2', 'true');
    await put('lasa.space.client.blocked.c3', '0');
    await put('lasa.space.client.blocked.c4', '1700000000');
    await put('lasa.space.role.blocked.noisy', '1700000000');
    await put('lasa.space.role.blocked.admin', '0'); // stored, no effect — still visible
    await put('lasa.space.channel.muted.backstage', 'true');
    await put('lasa.space.channel.gain.stage', '1.2');
    await put('lasa.space.channel.attenuation.whisper', '3');
    return s;
  }

  it('lists live entities in id order, ignoring marker-less subtrees and bad values', async () => {
    const view = new BaseView(await populated());
    const all = view.entities();
    expect(all.map((e) => e.id)).toEqual(['e1', 'e2']);
    const [e1, e2] = all;
    expect(e1).toEqual({
      id: 'e1',
      owner: 'c1',
      name: 'Alice',
      attrs: { colour: '00aaff', 'avatar.hat': true },
      signedAttrs: { tier: 'gold' },
      heardIn: ['main', 'stage'],
      hears: ['main'],
      priority: true,
      render: { gain: 1.5, attenuation: 2.3 },
      frame: 'world',
      dof: 3,
      mutes: ['e2'],
      solos: ['e3'],
    });
    expect(e2!.frame).toBe('head');
    expect(e2!.render).toEqual({});
    expect(e2!.dof).toBe(6);
    expect(e2!.priority).toBe(false);
  });

  it('reads one entity, and undefined for an absent marker', async () => {
    const view = new BaseView(await populated());
    expect(view.entity('e1')?.name).toBe('Alice');
    expect(view.entity('e9')).toBeUndefined();
    expect(view.entity('nope')).toBeUndefined();
  });

  it('reads moderation state', async () => {
    const view = new BaseView(await populated());
    expect(view.isEntityMuted('e2')).toBe(true);
    expect(view.isEntityMuted('e1')).toBe(false);
    expect(view.clientBlock('c3')).toEqual({ id: 'c3', until: null });
    expect(view.clientBlock('c4')).toEqual({ id: 'c4', until: new Date(1_700_000_000_000) });
    expect(view.clientBlock('c1')).toBeUndefined();
    expect(view.blockedClients().map((b) => b.id)).toEqual(['c3', 'c4']);
    expect(view.roleBlock('noisy')?.until).toEqual(new Date(1_700_000_000_000));
    expect(view.blockedRoles().map((b) => b.id)).toEqual(['admin', 'noisy']);
  });

  it('reads channel policy and the channel set', async () => {
    const view = new BaseView(await populated());
    expect(view.channel('backstage')).toEqual({ muted: true });
    expect(view.channel('stage')).toEqual({ muted: false, gain: 1.2 });
    expect(view.channel('whisper')).toEqual({ muted: false, attenuation: 3 });
    expect(view.channel('main')).toEqual({ muted: false });
    expect(view.channels()).toEqual(['backstage', 'main', 'stage', 'whisper']);
  });

  it('reflects a departure sweep: marker cleared → entity gone', async () => {
    const s = await populated();
    const view = new BaseView(s);
    await s.applyLive({ kind: 'clear', key: 'lasa.entity.e1', seq: 1000n });
    expect(view.entities().map((e) => e.id)).toEqual(['e2']);
    expect(view.entity('e1')).toBeUndefined();
  });
});
