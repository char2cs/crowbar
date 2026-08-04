//! `--surface sidebar-header` — `sidebar.tsx`'s `SidebarHeader`, drawn through
//! the **wrapped** `gpui_component::sidebar::SidebarHeader`.
//!
//! `crowbar_ui::surfaces::sidebar::shell` carries the division of labour, what had to
//! be overridden to reach it, and why the other two boxes in the vendor's
//! `sidebar/` module cannot be measured at all. This file is the cell.
//!
//! # `--body-height`, which is not a matrix axis
//!
//! `SidebarHeader` is a container: its one box is `flex flex-col gap-2 p-2` and
//! everything inside it belongs to the call site. The live one
//! (`file-explorer-tree.tsx`) holds a search `<Input>` and a filter `<Button>`,
//! two primitives this port already measures as their own surfaces — so the body
//! here is their **measured extent**, exactly as `popover`'s is, and the one box
//! that genuinely *is* this component compares against a reference whose
//! contents are whatever they happen to be.
//!
//! There is deliberately no `--body-width`. The header is a column whose
//! cross-axis alignment `sidebar.tsx` leaves at `normal`, so the child is
//! stretched: the live one measures 328 inside a 344 header without authoring a
//! width. `--width` alone decides both.
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real.** A `SidebarHeader` with no children is a picture this primitive genuinely has: the box collapses to its own `p-2`, so 344 × 44 becomes 344 × 16. It has no live reference — the file-explorer header always has its search row — so the caption says so. |
//! | `hover`, `selected` | **unmodelled, and this is the one surface where that word is load-bearing.** `sidebar.tsx`'s `SidebarHeader` carries no interaction rule at all — `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*'` over the component is empty — but the **vendor** paints a `sidebar_accent` fill for both, and those styles are applied *after* `refine_style` into a separate style map that no refinement reaches. So a cell driven on either flag would put `gpui-component`'s fill against React's nothing, by construction. Declaring them is what makes the binary say so on stderr instead. |
//! | `focus` | unmodelled: neither side has a focus rule on this box. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::sidebar::shell::{self, Header};
use gpui::{AnyElement, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-header",
    root: sidebar::ID_HEADER,
    // Five of the six. See the module docs for why `hover` and `selected` are
    // the interesting two: the React component has no such state and the vendor
    // does, which is the opposite of the usual reason a flag is unmodelled.
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live header is 44px; the caption sits below whatever `--body-height`
    // drives. A floor, not a ceiling — `driven_height` moves the window.
    min_window_height: 120,
    // A block in a sidebar panel, stretched by its parent rather than filling
    // the viewport.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--body-height`'s default: the live file-explorer header's single child,
/// measured off the running app at 28px — its 44px box less 8px of padding
/// either side.
pub const DEFAULT_BODY_HEIGHT: u16 = 28;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--body-height`: how tall `SidebarHeader`'s children come out.
    pub body_height: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            body_height: DEFAULT_BODY_HEIGHT,
        }
    }
}

impl Params {
    /// The header this cell describes.
    ///
    /// `empty` empties it: a `SidebarHeader` with no children is the box's own
    /// padding and nothing else.
    #[must_use]
    pub fn header(&self, cell: &Cell) -> Header {
        Header {
            body_height: if cell.has(StateFlag::Empty) {
                px(0.0)
            } else {
                px(f32::from(self.body_height))
            },
        }
    }

    /// The header's own height: the body plus two paddings.
    ///
    /// Spelled here rather than measured because [`SurfaceParams::driven_height`]
    /// is asked *before* a window exists — the window's size is what it decides.
    #[must_use]
    pub fn header_height(&self, cell: &Cell) -> u16 {
        let height = f32::from(self.header(cell).height());
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "the body is a `u16` and the padding is a small whole \
                      number of px; `Cell` needs a `u16` to stay `Eq`"
        )]
        {
            height.ceil() as u16
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--body-height" => self.body_height = pixels(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **The header's own height**, which `--body-height` decides — so the
    /// window follows it rather than capping it.
    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.header_height(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · body {}px", self.body_height);
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the live header always has its search row, so this cell has no reference",
            );
        }
    }

    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.header(cell).render(anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [(
        "--body-height <px>",
        format!("how tall SidebarHeader's children are [{DEFAULT_BODY_HEIGHT}]"),
    )]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_BODY_HEIGHT, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-header"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sidebar-header cell carries this surface's bag")
    }

    /// The defaults are the live file-explorer header, measured off the running
    /// app: a 44px box that is 8 + 28 + 8.
    #[test]
    fn the_defaults_are_the_live_file_explorer_header() {
        let bag = Params::default();

        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.header_height(&cell(&[])), 44);
        assert_eq!(bag.header(&cell(&[])).body_height, px(28.0));
    }

    /// **`empty` is the only flag this surface models**, and it moves the one
    /// box it has: a childless header is its padding and nothing else.
    #[test]
    fn empty_is_the_one_flag_that_moves_the_box() {
        let empty_cell = cell(&["--flags", "empty"]);
        let empty = params_of(&empty_cell);

        assert_eq!(empty.header_height(&empty_cell), 16);
        assert_eq!(empty.header(&empty_cell).body_height, px(0.0));
        assert_eq!(params_of(&cell(&[])).header_height(&cell(&[])), 44);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other five are declared unmodelled, which is what makes the binary
    /// name them on stderr rather than drawing a cell whose two sides disagree
    /// by construction — see the module docs for why `hover` and `selected` are
    /// the sharp ones here.
    #[test]
    fn the_five_interaction_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
            assert!(
                cell(&["--flags", flag.name()])
                    .unmodelled_flags()
                    .contains(&flag),
                "{flag:?} should be reported on stderr",
            );
        }
    }

    /// `--body-height` moves the box, and the window follows it — which is what
    /// stops a tall header being cut by a window sized for a short one.
    #[test]
    fn the_body_height_drives_the_box_and_the_window() {
        for (body, box_height) in [(0u16, 16u16), (28, 44), (120, 136), (400, 416)] {
            let driven = cell(&["--body-height", &body.to_string()]);
            assert_eq!(params_of(&driven).header_height(&driven), box_height);
            assert!(driven.window_extent() >= box_height);
        }
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--body-height", "tall"],
            vec!["--body-height"],
            vec!["--body-height", "-1"],
            vec!["--body-width", "328"],
            vec!["--tone", "error"],
        ] {
            let mut full = vec!["--surface", "sidebar-header"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// **`--body-height` belongs to this surface and to `popover`**, and to no
    /// third one — the property the registry exists for. `popover` spells it the
    /// same way for the same reason, and each cell only ever asks its own bag.
    #[test]
    fn this_surfaces_option_is_rejected_on_a_surface_that_does_not_take_it() {
        let line = ["--surface", "git-status-row", "--body-height", "28"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
        // …and it *is* accepted on `popover`, which is the same word meaning the
        // same thing rather than a collision.
        let shared = ["--surface", "popover", "--body-height", "28"];
        assert!(Cell::parse(shared.iter().map(|arg| (*arg).to_owned())).is_ok());
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        assert!(usage.contains("--body-height"));
        assert!(usage.contains("sidebar-header"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The caption names the cell that has **no reference to compare against**,
    /// and says nothing of the sort at the reachable default.
    #[test]
    fn the_caption_names_the_cell_that_has_no_reference() {
        assert!(
            cell(&["--flags", "empty"])
                .describe()
                .contains("no reference")
        );
        assert!(!cell(&[]).describe().contains("no reference"));
        assert!(cell(&[]).describe().contains("body 28px"));
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "sidebar-header");
        assert_eq!(SURFACE.root, "sidebar-header");
    }
}
