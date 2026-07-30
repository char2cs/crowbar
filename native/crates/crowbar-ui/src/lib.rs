#![forbid(unsafe_code)]

//! `crowbar-ui` — the design system: `Theme`, the sealed token newtypes and the
//! primitives that wrap `gpui-component`.
//!
//! Scaffold only (item 0.1). Awaiting the vendored `gpui` from item 0.2.
//!
//! Dependency contract (§4.2): `crowbar-core`, plus `gpui` and `gpui-component`
//! when they land.
//!
//! **Token sealing (§4.3 rule 3, §6.1).** `Color`, `Space`, `Radius`,
//! `FontSize` and `Duration` will have private inner fields and a `pub(crate)`
//! constructor only. No `from_raw`, no `pub const fn new`. Consumers write
//! `theme.surface.raised`; a colour or spacing literal at a call site outside
//! this crate is a compile error, not a review comment.
