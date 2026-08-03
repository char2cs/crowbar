//! `--surface sidebar-skeleton` — the sidebar's loading placeholder: two chat
//! rows, a divider, and two repo groups of a header row plus two workspace
//! rows.
//!
//! `crowbar_ui::components::sidebar_skeleton` carries the full account of
//! this composition's own eighteen bars and why it does not extend
//! `skeleton::CallSite`. This file is the cell — and an unusually thin one,
//! because [`SidebarSkeleton`] is a unit struct with no fields at all:
//! `sidebar-skeleton.tsx` hardcodes `[1, 2].map(...)` three times and takes
//! no prop of any kind, so every live cell of this surface is the identical
//! picture.
//!
//! # No reference — the composition is proven never to mount, not merely unobserved
//!
//! `crowbar_ui::components::sidebar_skeleton`'s own module docs record the
//! finding in full: `<SidebarSkeleton>`'s one call site is a `<Suspense
//! fallback={<SidebarSkeleton />}>` wrapping a statically-imported
//! `FileExplorerTree`, so the fallback never mounts on any build — a live
//! count of `[data-slot="skeleton"]` was **0** in every state, including
//! immediately after a full reload. There is no `/tmp/p3-ref-sidebar-skeleton.json`.
//! `web/src/lib/oracle/extract.ts`'s own test suite already carries the other
//! half of this same fact:
//! `expect(oracleSurfaceScope('sidebar-skeleton')).toBeNull()` — no scope
//! declaration exists or is needed, because every one of this composition's
//! eighteen bars plus its own divider is authored under this file's own
//! `sidebar-skeleton-*` namespace, with no foreign content nested inside it.
//!
//! # The state axis, and why this is a `no_state_axis` surface
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `sidebar-skeleton.tsx` renders no interaction rule of any kind — `select-none` is the closest thing, and it names nothing this contract compares. This is not a row, so `Empty` does not apply either: every one of the eighteen bars is unconditional, hardcoded by the two `[1, 2].map` calls the component takes no prop to bypass. |
//!
//! Checked exhaustively, the same standard `fps-overlay`'s and
//! `workspace-branch-icon`'s own module docs set: `export function
//! SidebarSkeleton()` takes **no props at all** — no `className`, no prop
//! spread, no field on the Rust side either. So there is no seam of any kind
//! left to reach through any hypothetical caller, and this surface's own
//! `Params` therefore carries no fields and no options.

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::sidebar_skeleton::{self, SidebarSkeleton};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-skeleton",
    root: sidebar_skeleton::ID_SIDEBAR_SKELETON,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // Two chat rows (36px each), a divider (~9px including its own margin)
    // and two repo groups of three 36px rows apiece (112px each), plus the
    // outer column's own py-1 and four 2px gaps between its five top-level
    // children, comes to 321px by hand; `row_layout::sidebar_skeleton`'s own
    // tests read the real number back rather than trusting this arithmetic.
    // 400 clears it with room and `CAPTION_HEIGHT`'s 29 besides, and is a
    // floor rather than a ceiling — this surface drives no height (nothing on
    // its command line could move it; there is no command line).
    min_window_height: 400,
    // The placeholder fills its own sidebar *column*, not the window — the
    // same ordinary inset `git-status-row`'s own surface takes.
    full_bleed: false,
    options,
    params: || Box::new(Params),
};

/// This surface's own options. **None** — see the module docs: the
/// composition takes no prop at all, so there is nothing here to drive.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Params;

impl SurfaceParams for Params {
    /// Never recognises a word — this surface has no options.
    fn accept(
        &mut self,
        _option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        Ok(false)
    }

    /// **None.** Nothing on this command line — there is nothing on this
    /// command line — moves the placeholder's own content-driven height.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        out.push_str(
            " · no parameters: sidebar-skeleton.tsx hardcodes every row and takes no \
             prop — every cell of this surface is the identical picture · no reference: \
             proven to never mount, see native/mapping/sidebar-skeleton.md",
        );
    }

    /// **`true`, unconditionally.** See the module docs: every §8.3 flag is
    /// checked and none applies — `sidebar-skeleton.tsx` takes no prop at
    /// all, so there is no branch, real or reachable-by-construction, left
    /// for a cell to drive on this surface.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let _ = cell; // no options to read; the same placeholder on every cell
        SidebarSkeleton.render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    Vec::new()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE};
    use crate::row_surface::{Cell, StateFlag};
    use crate::surface::SurfaceParams;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-skeleton"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// Any word this surface does not recognise as a shared option is a
    /// rejection — proving `accept` genuinely returns `Ok(false)` rather than
    /// silently swallowing everything.
    #[test]
    fn an_unrecognised_option_is_still_rejected() {
        let line = ["--surface", "sidebar-skeleton", "--not-a-real-option"];
        assert!(Cell::parse(line.iter().map(|arg| (*arg).to_owned())).is_err());
    }

    #[test]
    fn driven_height_is_none() {
        assert_eq!(Params.driven_height(&cell(&[])), None);
    }

    /// Every flag is unmodelled, and the surface declares that on purpose —
    /// the two facts `no_surface_declares_its_entire_state_axis_unmodelled`
    /// (`surface.rs`) requires to agree with each other.
    #[test]
    fn every_flag_is_unmodelled_and_the_surface_declares_it() {
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(Params.no_state_axis());
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "sidebar-skeleton");
        assert_eq!(SURFACE.root, "sidebar-skeleton");
        assert!(!SURFACE.full_bleed);
    }

    #[test]
    fn the_usage_names_this_surface() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("sidebar-skeleton"));
    }
}
