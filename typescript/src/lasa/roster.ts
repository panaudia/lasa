/**
 * roster.ts — the client half of the presence contract (lasa-core.md §7)
 * applied once, in the SDK: the index→entity mapping keyframes define,
 * the generation discard rule for deltas, per-index latest-wins by
 * datagram group-id, and the full tracking reset on generation change.
 * Feed every presence packet through apply() (LasaClient.subscribeRoster
 * wires this up); read the roster with snapshot().
 */

import type { Pose, PresenceKeyframe, PresenceDelta } from './wire.js';

/** One live entity as presence last reported it. */
export interface RosterEntry {
  id: string;
  headFrame: boolean;
  pose: Pose;
  loudness: number;
}

interface Slot {
  entry: RosterEntry;
  lastGroup: bigint; // group-id of the last applied record (latest-wins)
}

export class PresenceRoster {
  private gen = 0;
  private seen = false; // a keyframe has established a generation
  private slots = new Map<number, Slot>();

  /** Called after any packet that changed the roster. */
  onUpdate?: () => void;

  apply(groupId: bigint, msg: PresenceKeyframe | PresenceDelta): void {
    let changed = false;
    if (msg.kind === 'keyframe') {
      if (!this.seen || msg.gen !== this.gen) {
        // New generation: membership changed — every per-index tracking
        // state resets with it (§7), which is also how departure lands
        // (absence from the new roster).
        this.seen = true;
        this.gen = msg.gen;
        this.slots.clear();
        changed = true;
      }
      for (let i = 0; i < msg.records.length; i++) {
        const rec = msg.records[i]!;
        const idx = msg.first + i;
        const slot = this.slots.get(idx);
        if (!slot) {
          this.slots.set(idx, {
            entry: { id: rec.id, headFrame: rec.headFrame, pose: rec.pose, loudness: rec.loudness },
            lastGroup: groupId,
          });
          changed = true;
        } else if (groupId > slot.lastGroup) {
          // The mapping is constant within a generation; only the sampled
          // pose/loudness are gated by latest-wins, so a keyframe chunk
          // arriving behind a newer delta cannot rewind the pose.
          slot.entry.pose = rec.pose;
          slot.entry.loudness = rec.loudness;
          slot.lastGroup = groupId;
          changed = true;
        }
      }
    } else {
      if (!this.seen || msg.gen !== this.gen) return; // stale or pre-keyframe generation — discard (§7)
      for (const rec of msg.records) {
        const slot = this.slots.get(rec.index);
        if (!slot) continue; // index's keyframe chunk lost; next round covers it
        if (groupId > slot.lastGroup) {
          slot.entry.pose = rec.pose;
          slot.entry.loudness = rec.loudness;
          slot.lastGroup = groupId;
          changed = true;
        }
      }
    }
    if (changed) this.onUpdate?.();
  }

  /** The current roster, in unspecified order (entries are copies). */
  snapshot(): RosterEntry[] {
    return [...this.slots.values()].map((s) => ({ ...s.entry }));
  }
}
