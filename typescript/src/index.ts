/**
 * The public surface of `@panaudia/lasa-client`, the browser client for
 * LASA (live spatial audio over Media over QUIC).
 *
 * Start with {@link LasaClient.connect}. The guides in `docs/` cover
 * connecting, audio, presence, state, the base profile (mutes,
 * channels, moderation) and coordinate systems.
 * Internals (the transport codec, the worker protocol, the rings)
 * are not exported here and are reachable only by deep import.
 *
 * @packageDocumentation
 */

export {
  LasaClient,
  EntityPublisher,
  TRACK_MONO_OBJECT,
  type LasaClientOptions,
  type CaptureOptions,
  type CaptureHandle,
  type SinkFormat,
  type SinkIngress,
  type PresenceHandler,
  type SinkHandler,
} from './lasa/client.js';

export { PresenceRoster, type RosterEntry } from './lasa/roster.js';

export {
  type LasaEntity,
  type LasaSetup,
  type LasaConnectionConfig,
  TRACK_BINAURAL,
  TRACK_AMBI2,
  TRACK_AMBI3,
  TRACK_STATE,
  TRACK_PRESENCE,
} from './lasa/setup.js';

export {
  LASA_ERR,
  LasaRejectionError,
  isLasaRejectionCode,
  lasaErrorName,
} from './lasa/errors.js';

export {
  type Pose,
  type Redundancy,
  type MonoObjectPacket,
  type SinkPacket,
  type PresenceKeyframe,
  type PresenceDelta,
  type KeyframeRecord,
  type DeltaRecord,
  LOUDNESS_SILENT,
  loudnessToDBFS,
  MalformedError,
  UnknownFlagsError,
} from './lasa/wire.js';

export {
  threejsToLasa,
  lasaToThreejs,
  babylonToLasa,
  lasaToBabylon,
  aframeToLasa,
  lasaToAframe,
  playcanvasToLasa,
  lasaToPlaycanvas,
  unityToLasa,
  lasaToUnity,
  unrealToLasa,
  lasaToUnreal,
  pixiToLasa,
  lasaToPixi,
  type Vec3,
  type Vec2,
  type EulerOrder,
  type FRotator,
  type Vec3Pose,
  type UnrealPose,
  type PixiPose,
} from './lasa/coordinates.js';

export { Store, type KV, type StoreOptions } from './lasa/store.js';
export { StateSync, type StateSyncOptions } from './lasa/state.js';
export {
  EntityControls,
  SpaceControls,
  BaseView,
  baseView,
  keys as baseKeys,
  encodeValue,
  decodeValue,
  isIdentifier,
  BASE_PREFIX,
  type BaseValue,
  type StateWriter,
  type RenderParams,
  type EntityView,
  type BlockView,
} from './lasa/base.js';
export {
  type StateMessage,
  type SetOp,
  type ClearOp,
  type GroupOp,
  type FrontierMsg,
  type BeginMsg,
  type UpstreamStateMessage,
  type UpstreamSetOp,
  type UpstreamClearOp,
  type UpstreamGroupOp,
  type Cursor,
} from './lasa/statewire.js';

export {
  SinkPlayer,
  type SinkPlayerOptions,
  type SinkStatsListener,
  type AudioGraphReport,
  type PlayoutEvent,
  type SinkRingHandoff,
  probeOutputDeviceSampleRate,
} from './audio/player.js';
export { type SinkAudioConfig } from './worker/protocol.js';
export { CapturePipeline, type CapturePipelineOptions, type CaptureHandoff } from './audio/capture.js';
export {
  selectBestMicrophone,
  classifyByLabel,
  micPermissionGranted,
  type MicrophoneType,
  type ClassifiedMicrophone,
  type MicrophoneSelectionResult,
} from './audio/microphone-selection.js';
export { type JitterBufferSnapshot, type JitterBufferCoreConfig } from './audio/jitter-buffer-core.js';
export { type StereoMeterReport } from './audio/stereo-meter-core.js';
