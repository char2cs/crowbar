#![deny(unsafe_op_in_unsafe_fn)]

//! `crowbar-platform` — **the only crate in this workspace permitted to write
//! `unsafe`.**
//!
//! Scaffold only (item 0.1). macOS vibrancy, `NSWindow` manipulation,
//! reveal-in-Finder, `open_window` and `diagnostics_export` land here, ported
//! from `desktop/src-tauri` (§4.2, §10.1).
//!
//! Dependency contract (§4.2): `objc2`, macOS only. Nothing from the rest of
//! the workspace — this crate is a leaf so that the unsafe surface can be
//! audited without reading anything else.
//!
//! # The rule for this crate
//!
//! **Every `unsafe` block, `unsafe fn`, `unsafe impl` and `unsafe trait` in
//! this crate must carry a `# Safety` doc comment on its enclosing item, and
//! that comment must actually prove the obligation is discharged** — which
//! Objective-C selector is being sent, to what class, on which thread, with
//! what lifetime guarantees on every pointer that crosses the boundary.
//! "Safe because it works" is not a proof and will be rejected.
//!
//! `scripts/check-invariants.sh` fails the build if an `unsafe` construct
//! appears without a `# Safety` heading above its enclosing item. The script is
//! a floor, not a ceiling: it can see that the words are present, not that the
//! argument is sound. That part is the reviewer's job.
//!
//! Every other crate carries `#![forbid(unsafe_code)]` instead, which is a hard
//! compile error rather than a lint (§4.3 rule 2). This crate carries
//! `#![deny(unsafe_op_in_unsafe_fn)]` so that an `unsafe fn` body does not get
//! an implicit unsafe block for free — each operation must still be spelled
//! out and justified.
//!
//! # What is here now
//!
//! [`vibrancy`] (item S0.5, macOS only): the `HudWindow` blur behind
//! Crowbar's window and the appearance pin that keeps its frost per-theme.
//! The first `unsafe` this crate has held since P3.40 retired `native_menu` —
//! see [`vibrancy::pin_appearance`]'s doc comment for the proof.
//!
//! `native_menu` — a real macOS context menu (`NSMenu`), item P2.14 — lived
//! here and was retired at item P3.40: `native/oracle/blocked/
//! s13-native-menus-accepted-delta.md` retained it only until Phase 3 closed
//! "unless a concrete need appears that the vendored one cannot serve", P3.38
//! found no such need, and the module was wired to no call site — only to its
//! own `--surface native-menu` driver, which was retired with it. Context
//! menus in this port go through the vendored `gpui_component::native_menu`
//! instead, once something is wired to one (`native/mapping/context-menu.md`
//! §6).

#[cfg(target_os = "macos")]
mod vibrancy;

#[cfg(target_os = "macos")]
pub use vibrancy::{
    Inspection, Opacity, PinAppearanceError, RetuneError, VibrancyError, apply_vibrancy, inspect,
    pin_appearance, retune_blur,
};
