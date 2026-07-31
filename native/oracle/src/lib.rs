#![forbid(unsafe_code)]

//! `oracle` — the parity differ (spec §8.1).
//!
//! Two v1 anchor snapshots in — one from the React app, one from the GPUI app —
//! and **ranked deltas** out. That is the entire job. It does not drive apps,
//! does not take screenshots, and does not know what a component is. The
//! contract it implements is `native/oracle/ANCHORS.md`, in full; where this
//! crate has an opinion the contract does not state, the code says so at the
//! point of the decision.
//!
//! ```
//! use oracle::{Snapshot, Tolerances, diff};
//!
//! const REFERENCE: &str = r##"{
//!   "schema": 1, "surface": "git-status-row",
//!   "state": { "width": 320, "theme": "dark", "content": "short", "flags": [] },
//!   "root": "git-row-item",
//!   "anchors": [
//!     { "id": "git-row-item", "bounds": { "x": 0, "y": 0, "w": 320, "h": 24 },
//!       "bg": "#00000000", "visible": true },
//!     { "id": "git-row-icon", "bounds": { "x": 8, "y": 4, "w": 14, "h": 14 },
//!       "bg": "#00000000", "visible": true }
//!   ]
//! }"##;
//!
//! let reference = Snapshot::from_json(REFERENCE)?;
//! let native = Snapshot::from_json(&REFERENCE.replace(r#""x": 8"#, r#""x": 12"#))?;
//!
//! let report = diff(&reference, &native, &Tolerances::V1)?;
//! assert_eq!(
//!     report.deltas.first().map(ToString::to_string),
//!     Some("git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)".to_owned())
//! );
//! # Ok::<(), Box<dyn std::error::Error>>(())
//! ```
//!
//! # What this oracle is, exactly
//!
//! Repeating ANCHORS.md §6, because a reader will otherwise assume it covers
//! more than it does: **this is a geometry-and-colour oracle.** Shadows, blur,
//! `backdrop-filter`, vibrancy, transitions, compositing and paint order have
//! no representation in a snapshot and therefore none here. Those belong to the
//! secondary perceptual oracle (§8.2), whose output is a human-triaged score.
//! Do not claim otherwise in any report.
//!
//! # Dependency contract (§4.2)
//!
//! **None.** Not on the rest of the workspace, not on `gpui`, and not on
//! `serde_json` — the JSON reader in [`json`] is hand-rolled for that reason.
//! The oracle judges the app; it must not be able to import the thing it
//! judges.
//!
//! # Layout
//!
//! - [`json`] — a strict, dependency-free JSON reader.
//! - [`color`] — `#rrggbbaa`, parsed strictly.
//! - [`snapshot`] — the §2–§4 model and its unforgiving loader.
//! - [`tolerance`] — the §5 table, as data, in one place.
//! - [`delta`] — what a disagreement is, how severe it is, and how it reads.
//! - [`diff`] — the comparison and the ranking.
//! - [`cli`] — `--report`, and the exit codes CI gates on.
//!
//! The logic lives in the library rather than in `main.rs` because §12 holds
//! this crate to ≥98% line coverage, which needs a library target to measure.
//!
//! `corpus/` and `blocked/` sit alongside `src/` and are **not** part of the
//! crate. `corpus/` is append-only and ratcheted (§8.4); worker agents may not
//! write to either.

pub mod cli;
pub mod color;
pub mod delta;
pub mod diff;
pub mod json;
pub mod snapshot;
pub mod tolerance;

pub use color::Color;
pub use delta::{Class, Delta, DeltaKind};
pub use diff::{
    ContentSizing, ContextField, ContextMismatch, Contributor, Forgiven, Forgiveness, Report, diff,
};
pub use snapshot::{Anchor, Border, Bounds, Font, LoadError, LoadErrorKind, Snapshot, State};
pub use tolerance::Tolerances;
