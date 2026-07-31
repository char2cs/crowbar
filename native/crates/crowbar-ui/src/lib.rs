#![forbid(unsafe_code)]

//! `crowbar-ui` — the design system: `Theme`, the sealed token newtypes and the
//! primitives that wrap `gpui-component`.
//!
//! Scaffold plus the framework wiring from item 0.4. The `Theme` and the sealed
//! tokens themselves are still to come.
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
//! `FontSize` and `Duration` will have private inner fields and a `pub(crate)`
//! constructor only. No `from_raw`, no `pub const fn new`. Consumers write
//! `theme.surface.raised`; a colour or spacing literal at a call site outside
//! this crate is a compile error, not a review comment.

pub use gpui;
pub use gpui_component;
