[@panaudia/lasa-client](../index.md) / StateSync

# Class: StateSync

Defined in: [src/lasa/state.ts:31](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/state.ts#L31)

Runs the state subscription loop until stop() is called. All store
mutation happens on a single promise chain, preserving per-lane
arrival order.

## Methods

### run()

```ts
run(): Promise<void>;
```

Defined in: [src/lasa/state.ts:55](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/state.ts#L55)

Starts the subscribe/apply/re-subscribe loop; resolves on stop().

#### Returns

`Promise`\<`void`\>

***

### settled()

```ts
settled(): Promise<void>;
```

Defined in: [src/lasa/state.ts:89](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/state.ts#L89)

Resolves after every message applied so far has been processed.

#### Returns

`Promise`\<`void`\>

***

### stop()

```ts
stop(): void;
```

Defined in: [src/lasa/state.ts:78](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/state.ts#L78)

Ends the loop: unregisters the lane, ends the current instance, and wakes any backoff wait.

#### Returns

`void`
