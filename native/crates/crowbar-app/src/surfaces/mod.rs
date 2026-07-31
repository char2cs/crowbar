//! One file per measurable surface, and **no list**.
//!
//! There is deliberately no `mod git_status_row;` here. `build.rs` reads this
//! directory, writes a `#[path]` module declaration per `.rs` file and a
//! [`ALL`] slice over their `SURFACE` statics, and the `include!` below pulls
//! the result in. Adding a surface is adding a file; two surfaces landing from
//! two worktrees at once cannot conflict, because neither of them wrote the
//! list.
//!
//! `build.rs` carries the reasoning and the costs. The one worth repeating here
//! is that `git grep 'mod dropdown_menu'` finds nothing — this comment is the
//! answer to that.
//!
//! # Adding one
//!
//! Write `src/surfaces/<name>.rs` with:
//!
//! ```ignore
//! pub static SURFACE: Surface = Surface { name: "…", root: "…", … };
//!
//! #[derive(Clone, Debug, PartialEq)]
//! pub struct Params { /* this surface's options, and only its own */ }
//!
//! impl SurfaceParams for Params { fn accept(…) fn describe(…) fn render(…) }
//! ```
//!
//! Nothing else. Not a `mod` line, not a dispatch arm, not a field on `Cell`.

include!(concat!(env!("OUT_DIR"), "/surfaces.rs"));
