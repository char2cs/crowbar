#![forbid(unsafe_code)]

//! `crowbar-diff` — the native review surface.
//!
//! View surface itself is still scaffold-only (item 0.1); [`review_placeholder`]
//! (P3.79) is this crate's first landed logic module.
//!
//! Dependency contract (§4.2): `crowbar-ui`, `crowbar-state`, `crowbar-core`.
//!
//! §12 splits this crate's gate in two: the diff *logic* is held to ≥98% line
//! coverage, the *view* to the oracle corpus.
//!
//! **Correction (P3.79):** this doc comment previously said "keep the
//! algebra in `crowbar-core` … and keep this crate to rendering and
//! interaction." That was this crate's own first guess, written before any
//! of §12's "diff(logic)" partition had a concrete example to test it
//! against. `native/mapping/tier-a-denominator.md`'s later, per-function
//! export-level audit (§1/§2) found a real example — the windowed review
//! renderer's placeholder-hunk-geometry estimation — and placed it here
//! instead, reasoning that it exists purely to size *this* crate's own
//! virtualiser, shares no consumer with `crowbar-core`'s other diff-model
//! work, and is exactly what the coverage-gate table's "logic ≥98%" column
//! was reserved for. See [`review_placeholder`]'s module doc for the full
//! citation trail, including where that finding disagrees with spec §4.2's
//! crate-contracts table (which lists "diff algebra" under `crowbar-core`
//! with no carve-out) and why this item followed the later, narrower
//! analysis rather than the table's literal wording.
//!
//! §10.1: check whether the daemon already returns unified diff before
//! reaching for `imara-diff`. If it does, `features/git/utils/
//! git-diff-parser.ts` ports directly and no algorithm is needed. (Checked,
//! P3.79: it does — see [`review_placeholder`]'s module doc for the finding
//! this resolves to, and why the real patch-text parsing that DOES happen
//! today, `@pierre/diffs`'s `parsePatchFiles`, is not reimplemented here.)

pub mod review_placeholder;
