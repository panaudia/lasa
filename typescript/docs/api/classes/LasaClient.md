[@panaudia/lasa-client](../index.md) / LasaClient

# Class: LasaClient

Defined in: [src/lasa/client.ts:194](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L194)

A connection to one LASA space: capture, playout, presence and
state over a single WebTransport session. Create one with
[LasaClient.connect](#connect); end it with [LasaClient.close](#close).

## Properties

| Property | Modifier | Type | Description | Defined in |
| ------ | ------ | ------ | ------ | ------ |
| <a id="space"></a> `space` | `readonly` | [`SpaceControls`](SpaceControls.md) | Space-wide base-profile operations — moderation, channel policy, rendering (see [SpaceControls](SpaceControls.md)). Role-gated by the server. | [src/lasa/client.ts:224](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L224) |

## Methods

### close()

```ts
close(): Promise<void>;
```

Defined in: [src/lasa/client.ts:589](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L589)

Ends the session: stops every capture and sink, stops the state
sync, and tears down the worker and transport. Idempotent.

#### Returns

`Promise`\<`void`\>

***

### controls()

```ts
controls(entityId): EntityControls;
```

Defined in: [src/lasa/client.ts:344](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L344)

Owner-scope base-profile controls for one of this connection's
entities, without waiting for its publisher. The same operations
are on [EntityPublisher](EntityPublisher.md).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

[`EntityControls`](EntityControls.md)

***

### entity()

```ts
entity(entityId, timeoutMs?): Promise<EntityPublisher>;
```

Defined in: [src/lasa/client.ts:352](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L352)

The entity's publisher; resolves when the server has subscribed
the entity's source track (normally within the first RTTs).

#### Parameters

| Parameter | Type | Default value |
| ------ | ------ | ------ |
| `entityId` | `string` | `undefined` |
| `timeoutMs` | `number` | `5000` |

#### Returns

`Promise`\<[`EntityPublisher`](EntityPublisher.md)\>

***

### getCaptureOverflows()

```ts
getCaptureOverflows(entityId): number | null;
```

Defined in: [src/lasa/client.ts:502](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L502)

Capture-ring overflows for a capturing entity (each = a dropped quantum = an upstream gap), or null.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`number` \| `null`

***

### getSinkIngress()

```ts
getSinkIngress(entityId): SinkIngress | null;
```

Defined in: [src/lasa/client.ts:531](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L531)

Latest ingress health for a playing sink (updated ~2 s by the
worker), or null before the first report. Distinguishes network
damage (gaps/loss/reorder — clicks that are NOT the jitter
buffer's) from buffer behaviour (the player's getEvents()).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

[`SinkIngress`](../interfaces/SinkIngress.md) \| `null`

***

### playSink()

```ts
playSink(
   entityId, 
   format, 
opts?): Promise<SinkPlayer>;
```

Defined in: [src/lasa/client.ts:390](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L390)

Subscribes an entity's sink track and plays it through the 3-thread
playout path: worker decode → SAB jitter ring → playout worklet.
Main never touches the audio. Returns the SinkPlayer (volume,
jitter stats, Tap B); call [stopSink](#stopsink) or close() to end it.
One player per entity sink (multi-entity: call per entity).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `format` | [`SinkFormat`](../type-aliases/SinkFormat.md) |
| `opts` | [`SinkPlayerOptions`](../interfaces/SinkPlayerOptions.md) |

#### Returns

`Promise`\<[`SinkPlayer`](SinkPlayer.md)\>

***

### startCapture()

```ts
startCapture(entityId, opts): Promise<CaptureHandle>;
```

Defined in: [src/lasa/client.ts:450](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L450)

Starts an entity's capture: any AudioNode (mic source, oscillator,
DAW channel) feeds a mono pipeline — worklet → SAB ring → worker
Opus encode → mono-object datagrams stamped with the FRESHEST pose
from the shared cell (update it via [CaptureHandle.setPose](../interfaces/CaptureHandle.md#setpose)).
Per-entity: call once per publishing entity, sharing one context.

The source's AudioContext MUST run at 48 kHz (lasa-core.md §5: Opus,
48 kHz, 5 ms) — construct it with `new AudioContext({ sampleRate:
48000 })`; any other rate is refused. Frames are always 5 ms.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `opts` | [`CaptureOptions`](../interfaces/CaptureOptions.md) |

#### Returns

`Promise`\<[`CaptureHandle`](../interfaces/CaptureHandle.md)\>

***

### stopCapture()

```ts
stopCapture(entityId): Promise<void>;
```

Defined in: [src/lasa/client.ts:507](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L507)

Stops an entity's capture: flushes the worker encoder, removes the graph nodes.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`Promise`\<`void`\>

***

### stopSink()

```ts
stopSink(entityId): Promise<void>;
```

Defined in: [src/lasa/client.ts:516](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L516)

Stops a playing sink: worker decode ends, then the graph tears down.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |

#### Returns

`Promise`\<`void`\>

***

### subscribePresence()

```ts
subscribePresence(handler): Promise<void>;
```

Defined in: [src/lasa/client.ts:560](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L560)

Subscribes the space presence track. The handler receives each
packet's datagram group-id — part of the §7 receive contract
(per-index latest-wins). Most consumers want subscribeRoster,
which applies the §7 rules once and maintains the roster.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `handler` | [`PresenceHandler`](../type-aliases/PresenceHandler.md) |

#### Returns

`Promise`\<`void`\>

***

### subscribeRoster()

```ts
subscribeRoster(onUpdate?): Promise<PresenceRoster>;
```

Defined in: [src/lasa/client.ts:578](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L578)

Subscribes presence and maintains a PresenceRoster from it — the
§7 client rules (generation discard, per-index latest-wins, reset
on generation change) applied in the SDK.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `onUpdate?` | () => `void` |

#### Returns

`Promise`\<[`PresenceRoster`](PresenceRoster.md)\>

***

### subscribeSink()

```ts
subscribeSink(
   entityId, 
   format, 
handler): Promise<void>;
```

Defined in: [src/lasa/client.ts:538](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L538)

Subscribes an entity's sink track (own-or-granted per §3.1/§2).

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `entityId` | `string` |
| `format` | [`SinkFormat`](../type-aliases/SinkFormat.md) |
| `handler` | [`SinkHandler`](../type-aliases/SinkHandler.md) |

#### Returns

`Promise`\<`void`\>

***

### syncState()

```ts
syncState(store, opts?): StateSync;
```

Defined in: [src/lasa/client.ts:319](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L319)

Runs the state subscription against the store: subscribe with the
store's cursor, feed both lanes, re-subscribe with backoff on
termination. Returns the running StateSync; runs until stop() or
close(). All store access from the caller should await
sync.settled() first.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `store` | [`Store`](Store.md) |
| `opts` | [`StateSyncOptions`](../interfaces/StateSyncOptions.md) |

#### Returns

[`StateSync`](StateSync.md)

***

### writeState()

```ts
writeState(...msgs): Promise<void>;
```

Defined in: [src/lasa/client.ts:308](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L308)

Publishes client-authored state ops on the state source subgroup
(upstream layout — no seqs). The worker waits for the server's
subscribe of the state source if it has not landed yet.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| ...`msgs` | [`UpstreamStateMessage`](../type-aliases/UpstreamStateMessage.md)[] |

#### Returns

`Promise`\<`void`\>

***

### connect()

```ts
static connect(opts): Promise<LasaClient>;
```

Defined in: [src/lasa/client.ts:241](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/client.ts#L241)

Spawns the MOQ worker, connects, authenticates via the Connection
Config, and wires the per-entity publishers as the server
subscribes them back.

#### Parameters

| Parameter | Type |
| ------ | ------ |
| `opts` | [`LasaClientOptions`](../interfaces/LasaClientOptions.md) |

#### Returns

`Promise`\<`LasaClient`\>
