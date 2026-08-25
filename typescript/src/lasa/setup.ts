/**
 * LASA connection setup (lasa-core.md §4.1): the client authenticates
 * with exactly one message — a Connection Config carried as a setup
 * parameter on CLIENT_SETUP. There is no per-subscribe authorization;
 * every legal track tuple derives from this config's client and entity
 * ids, and the server enforces the binding thereafter.
 */

import type { Kvp } from '../transport/moq-transport.js';

/** Setup parameter key 0x1A5A1 (odd → length-prefixed bytes). */
export const SETUP_PARAM_CONNECTION_CONFIG = 0x1a5a1;

/** An entity definition (schemata/entity.schema.json subset). */
export interface LasaEntity {
  /** Entity id, unique within the space. An identifier per `lasa-core.md` §2. */
  id: string;
  /** Display name. */
  name: string;
  /** Priority entity (profile semantics, `lasa-base-profile.md`). */
  priority?: boolean;
  /** Channels in which this entity's source is heard (§4; omitted → ["main"]). */
  'heard-in'?: string[];
  /** Channels this entity's sink listens to (§4; omitted → ["main"]). */
  hears?: string[];
  /** Free-form application attributes, published in state. */
  attrs?: Record<string, unknown>;
  /** Pose frame: `world` (default) or `head` (`lasa-core.md` §3). */
  frame?: 'world' | 'head';
  /** Degrees of freedom the entity's pose carries (default 6). */
  dof?: 0 | 3 | 6;
  /**
   * Latency-tolerance intent for this entity's audio path, 0–2
   * (lasa-core.md §4.2; path-scoped: the space applies it to its
   * ingest buffer, the client applies it to its own playout buffer).
   * 0 = interactive (default), 1 = resilient, 2 = playback/broadcast.
   */
  quality?: 0 | 1 | 2;
  /**
   * Maximum redundancy offset 0–7 for this entity's path (§4.2/§5.1);
   * bounds the entity's uplink emission and requests sink-track
   * redundancy from the space. 0 (default) = none.
   */
  redundancy?: number;
  'initial-pose'?: {
    position?: { x: number; y: number; z: number };
    attitude?: { yaw: number; pitch: number; roll: number };
  };
  /** Rendering parameters: gain multiplier, distance attenuation, source size, directivity. */
  render?: { gain?: number; attenuation?: number; size?: number; directivity?: number };
}

/** A setup instantiating a ticket-defined entity by id. */
export interface LasaSetup {
  /** Id of an entity the ticket defines. */
  id: string;
  /** Attributes merged over the ticket's definition. */
  attrs?: Record<string, unknown>;
  'initial-pose'?: LasaEntity['initial-pose'];
}

/** The Connection Config (schemata/connection.schema.json). */
export interface LasaConnectionConfig {
  clientId: string;
  /** Signed JWT ticket; omit only against a space allowing unticketed. */
  ticket?: string;
  /** Ad-hoc entity definitions (unticketed, or presenter/admin tickets). */
  entities?: LasaEntity[];
  /** Instantiations of ticket-defined entities. */
  setups?: LasaSetup[];
}

const textEncoder = new TextEncoder();

/** Builds the CLIENT_SETUP parameter carrying the Connection Config. */
export function connectionConfigParam(cfg: LasaConnectionConfig): Kvp {
  const json: Record<string, unknown> = { client_id: cfg.clientId };
  if (cfg.ticket !== undefined && cfg.ticket !== '') {
    json['ticket'] = cfg.ticket;
  }
  if (cfg.entities !== undefined) {
    json['entities'] = cfg.entities;
  }
  if (cfg.setups !== undefined) {
    json['setups'] = cfg.setups;
  }
  return {
    type: SETUP_PARAM_CONNECTION_CONFIG,
    value: textEncoder.encode(JSON.stringify(json)),
  };
}

/** LASA namespace tuples (lasa-core.md §3). */
export function entityNamespace(space: string, entityId: string, direction: 'source' | 'sink'): string[] {
  return [space, 'entity', entityId, direction];
}
export function clientNamespace(space: string, clientId: string, direction: 'source' | 'sink'): string[] {
  return [space, 'client', clientId, direction];
}
export function presenceNamespace(space: string): string[] {
  return [space, 'presence'];
}

/** Track names. */
export const TRACK_MONO_OBJECT = 'mono-object';
export const TRACK_BINAURAL = 'binaural';
export const TRACK_AMBI2 = 'ambi2';
export const TRACK_AMBI3 = 'ambi3';
export const TRACK_STATE = 'state';
export const TRACK_PRESENCE = 'presence';
