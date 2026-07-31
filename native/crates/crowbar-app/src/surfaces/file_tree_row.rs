//! `--surface file-tree-row` — the Phase 1 **state** gate.
//!
//! One row of the file explorer tree. It exists because the git status row's
//! state axis is vacuous: `SidebarTreeRow` takes an `active` prop no live
//! consumer passes, so `data-active` never fires there and every `selected` cell
//! compares resting against resting. This row sets `data-active` on every render
//! **and** sits inside `.file-tree-container`, which brings the scoped
//! `:focus-visible` border rule into play as well.
//!
//! # Its options are still fields on `Cell`, and that is the exception
//!
//! `--depth`, `--prev-depth`, `--next-depth` and `--git-status` are parsed by
//! the shared parser and stored on [`Cell`], which is why [`Params`] below is
//! empty. See `crate::surface`'s module docs. **A new surface does not copy
//! this.**

use crowbar_ui::Theme;
use crowbar_ui::components::{AnchorSink, file_tree_row};
use gpui::AnyElement;
use std::fmt::Write as _;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "file-tree-row",
    root: file_tree_row::ID_ITEM,
    // `empty` joins the other two here: a file explorer row always paints an
    // icon and a name and has no optional content to remove, where the git
    // row's trailing badge and counts are optional.
    unmodelled: &[StateFlag::Empty, StateFlag::Loading, StateFlag::Error],
    // One row and a caption, and — because this surface drives no height (see
    // `driven_height`) — the number its window still is, exactly.
    min_window_height: 72,
    options,
    params: || Box::new(Params),
};

/// This surface's own options: **none**. See the module docs.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Params;

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        _option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        Ok(false)
    }

    /// **None**, for the reason `git-status-row`'s is: a row is one line and
    /// nothing on this command line moves its height.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// `--git-status` is named only here: `GitFileItem` pins its filename at
    /// `text-foreground` and never moves it, so a git status row announcing
    /// `git modified` would be describing a colour nothing painted.
    fn describe(&self, cell: &Cell, out: &mut String) {
        if let Some(status) = cell.git_status {
            let _ = write!(out, " · git {}", status.name());
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        cell.file_row().render(theme, anchors)
    }
}

/// Advertised in the shared option list rather than here — see the module docs.
fn options() -> Vec<(String, String)> {
    Vec::new()
}
