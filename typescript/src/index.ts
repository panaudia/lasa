/**
 * @panaudia/lasa-client — public surface. Everything an application
 * needs; internals (transport codec, worker protocol, rings) stay
 * unexported here and reachable only by deep import.
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
  type MonoObjectPacket,
  type SinkPacket,
  type PresenceKeyframe,
  type PresenceDelta,
  type KeyframeRecord,
  type DeltaRecord,
  LOUDNESS_SILENT,
  loudnessToDBFS,
} from './lasa/wire.js';

export { Store, MemPersistence, type Persistence, type Snapshot, type KV } from './lasa/store.js';
export { StateSync, type StateSyncOptions } from './lasa/state.js';
export { type StateMessage, type Cursor } from './lasa/statewire.js';

export {
  SinkPlayer,
  type SinkPlayerOptions,
  type SinkStatsListener,
  type AudioGraphReport,
  type PlayoutEvent,
  probeOutputDeviceSampleRate,
} from './audio/player.js';
export { CapturePipeline, type CapturePipelineOptions, type CaptureHandoff } from './audio/capture.js';
export {
  selectBestMicrophone,
  classifyByLabel,
  micPermissionGranted,
  type MicrophoneType,
  type ClassifiedMicrophone,
  type MicrophoneSelectionResult,
} from './audio/microphone-selection.js';
export { type JitterBufferSnapshot } from './audio/jitter-buffer-core.js';
export { type StereoMeterReport } from './audio/stereo-meter-core.js';
