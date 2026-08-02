#![forbid(unsafe_code)]

//! `crowbar-ui` — the design system: `Theme`, the sealed token newtypes and the
//! primitives that wrap `gpui-component`.
//!
//! Dependency contract (§4.2): `crowbar-core`, `gpui`, `gpui-component`.
//!
//! **The re-exports below are load-bearing.** §4.2 puts `crowbar-terminal`,
//! `-editor`, `-diff` and `-webview` downstream of this crate and gives them
//! `ui` + `state` (+ `core`) and nothing else — they are meant to reach the
//! framework *through* the design system, so that a future change of framework
//! version, or a wrapper interposed in front of a `gpui-component` type, is one
//! edit here rather than one per leaf crate. Without these lines that contract
//! is unimplementable and each leaf would have to restate the dependency.
//!
//! **Token sealing (§4.3 rule 3, §6.1).** `Color`, `Space`, `Radius`,
//! `FontSize` and `Duration` have private inner fields and a
//! `pub(in crate::theme)` constructor only — stricter than `pub(crate)`, so not
//! even the rest of this crate can mint one. No `from_raw`, no `pub const fn
//! new`. Consumers write `theme.background`; a colour or spacing literal at a
//! call site outside this crate is a compile error, not a review comment.
//!
//! The seal is necessary and not sufficient, and the gap is the re-export
//! directly below: `crowbar_ui::gpui::rgb(0x1e1e1e)` is reachable from every
//! view crate, and `.bg(rgb(…))` on a raw gpui element never touches [`Theme`].
//! No private field can prevent that — a view crate cannot render without
//! gpui's colour types in scope — so `scripts/check-invariants.sh` rule 4 bans
//! raw colour construction outside `src/theme/`. The two together are the
//! §4.3 rule 3 guard; either alone is a suggestion.

pub mod components;
pub mod theme;

pub use components::{AnchorSink, GitStatusRow, RowState, Unanchored};
pub use gpui;
pub use gpui_component;
pub use theme::{
    Appearance, Color, Duration, FontFamily, FontSize, Radius, Scale, Space, Theme, ui_sans_font,
};
