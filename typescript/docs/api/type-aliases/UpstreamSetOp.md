[@panaudia/lasa-client](../index.md) / UpstreamSetOp

# Type Alias: UpstreamSetOp

```ts
type UpstreamSetOp = Omit<SetOp, "seq"> & object;
```

Defined in: [src/lasa/statewire.ts:57](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/statewire.ts#L57)

What a client writes: the same ops without the server-assigned seq
(the upstream layout never encodes it). A downstream StateMessage is
assignable here, so decoded messages can be re-published unchanged.

## Type Declaration

| Name | Type | Defined in |
| ------ | ------ | ------ |
| `seq?` | `bigint` | [src/lasa/statewire.ts:57](https://github.com/panaudia/lasa/blob/main/typescript/src/lasa/statewire.ts#L57) |
