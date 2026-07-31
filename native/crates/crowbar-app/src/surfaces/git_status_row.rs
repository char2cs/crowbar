//! `--surface git-status-row` — the Phase 1 **geometry** gate.
//!
//! One row of the git status panel, whose filename and directory truncate
//! against each other through three nested flex containers. That is where a
//! taffy layout is most likely to disagree with `WebKit`, which is why it was
//! the row the Phase 1 gate was measured on.
//!
//! # Its options are still fields on `Cell`, and that is the exception
//!
//! `--depth`, `--prev-depth`, `--next-depth`, `--added`, `--deleted`,
//! `--no-directory`, `--no-icon` and `--compact` are parsed by the shared
//! parser and stored on [`Cell`], not in [`Params`] below — which is why
//! [`Params`] is empty. See `crate::surface`'s module docs: the two Phase 1
//! surfaces stayed put because the assertions that name those fields are the
//! archived gate's evidence, and rewriting evidence to tidy a layout is not a
//! trade worth making. **A new surface does not copy this.**

use crowbar_ui::Theme;
use crowbar_ui::components::{AnchorSink, git_status_row};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "git-status-row",
    root: git_status_row::ID_ITEM,
    // Neither original has a `loading` or an `error` rendering of the row
    // itself — a gap in the thing being ported, not in the port. `empty` is
    // real here and only here: this row's trailing badge and counts are
    // optional, where a file explorer row always paints an icon and a name.
    unmodelled: &[StateFlag::Loading, StateFlag::Error],
    // One row and a caption. The number the archived gate runs were taken at,
    // and — because this surface drives no height (see `driven_height`) — the
    // number its window still is, exactly.
    min_window_height: 72,
    // A row inside the git panel's sidebar column, never the window itself. The
    // inset it therefore keeps is the geometry its archived gate runs were taken
    // at.
    full_bleed: false,
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

    /// **None.** A row is one line of a fixed height and no option on this
    /// command line moves it, so there is nothing for the window to follow and
    /// the floor is the whole answer. That is what keeps this surface's window
    /// at the geometry its archived Phase 1 runs were taken at.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// The two row parameters the *other* surfaces have no prop for, each named
    /// only where it is honoured.
    fn describe(&self, cell: &Cell, out: &mut String) {
        if !cell.show_directory {
            out.push_str(" · no-directory");
        }
        if !cell.show_file_icon {
            out.push_str(" · no-icon");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        cell.row().render(theme, anchors)
    }
}

/// Advertised in the shared option list rather than here, because the shared
/// parser is what reads them — see the module docs.
fn options() -> Vec<(String, String)> {
    Vec::new()
}
