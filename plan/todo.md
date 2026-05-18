# LASA — TODO and Open Questions

> **WIP reminder.** This is a single-author draft. See [`../README.md`](../README.md) for context on what LASA is, why it exists, and how it relates to existing standards.

## What's done

- [x] Draft v1 spec — packet layout, NodeState, TLV extensions, fragmentation, packet types, codec FourCCs, format enum, validation rules, versioning policy, worked example. See [`../lasa.md`](../lasa.md).

## What's needed for a real v1

### Validation

- [ ] **Reference implementation.** A Rust encoder/decoder that round-trips canonical packets, ideally with property-based fuzz testing. Currently being developed in the [panther](../../../rust/panther) repo as the panaudia internal mixer.
- [ ] **Cross-language ports** to exercise the spec from a non-author viewpoint. TypeScript, C++, and ideally one more (Python or Go). The act of porting reliably surfaces under-specified corners.
- [ ] **Canonical test vectors** in `tests/wire_vectors.json` — hex-encoded packets covering edge cases (min size, fragmentation, TLVs, unknown TLV skip, malformed inputs that must be rejected).
- [ ] **Independent spec review.** Someone other than the author reading critically and asking pointed questions.

### Spec maturity

- [ ] **`conventions.md`** — small companion doc for the shared UUID/coordinate/unit conventions used across the panaudia stack. Both this spec and any sibling specs (slow-plane state sync) link to it.
- [ ] **TLV registry stub.** v1 has no standardized TLV types defined. As real use cases appear (priority hints, sender clock epoch, reverb sends, HRTF profile IDs), document them in a registry section.
- [ ] **Format and codec registries.** Same — the v1 enums are starter sets; reserve space and define an addition process.
- [ ] **Stability commitment.** Decide when v1 is declared frozen, and what process applies to additive changes vs. breaking changes thereafter.

### Governance

- [ ] **Spec text licensing.** The repo has a `LICENSE` file but the spec doc itself should state its license explicitly. The spec text should be permissively licensed (CC-BY or similar) so implementations can quote it freely.
- [ ] **Trademark / name policy.** "LASA" as a name has no formal claim. If the spec gets adoption, decide whether to register the name and define a conformance/branding policy, or keep it informal.
- [ ] **Versioning policy.** §13 of the spec sketches this — additive via TLVs and new enum values without version bump, breaking via version increment. Worth formalizing into a separate policy doc as adoption grows.
- [ ] **Contribution / change process.** Right now it's one author. If others want to contribute, define how proposals are made and accepted (RFC process? Issues? Pull requests with rationale?).

## Open design questions

These are deferred or unresolved and could affect v1 if revisited:

- [ ] **Whether `space_id` is needed on every packet** given MOQ track scoping. Current spec keeps it for self-describing portability across transports, but a leaner profile for internal hops could be valuable.
- [ ] **Whether to define a `BUNDLE` packet type** for multi-source aggregation at edge relays. `packet_type = 4` is reserved for it; the design is sketched in conversation notes but not specified.
- [ ] **Whether to standardize a sender-clock epoch TLV** so receivers can detect sender restarts cleanly (vs. inferring from `sample_ts` going backwards).
- [ ] **Fragmentation reassembly timeout policy.** Currently implementation-defined. May want a recommended default once measured under realistic network conditions.
- [ ] **Recommended profiles.** "LASA-mono-PCM," "LASA-FOA-Opus," "LASA-HOA3-PCM-fragmented" — named subsets that simple implementations can target without supporting the full spec surface.
