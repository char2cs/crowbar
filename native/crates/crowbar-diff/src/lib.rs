#![forbid(unsafe_code)]

//! `crowbar-diff` — the native review surface.
//!
//! Scaffold only (item 0.1).
//!
//! Dependency contract (§4.2): `crowbar-ui`, `crowbar-state`, `crowbar-core`.
//!
//! §12 splits this crate's gate in two: the diff *logic* is held to ≥98% line
//! coverage, the *view* to the oracle corpus. Keep the algebra in
//! `crowbar-core` where the line gate already applies, and keep this crate to
//! rendering and interaction.
//!
//! §10.1: check whether the daemon already returns unified diff before
//! reaching for `imara-diff`. If it does, `features/git/utils/
//! git-diff-parser.ts` ports directly and no algorithm is needed.
