//! `--surface tooltip` — the item's finding that `tooltip.tsx` fails the wrap
//! test `popover` set, drawn as a cell.
//!
//! `crowbar_ui::components::tooltip` carries the seam evidence and the
//! reachability measurement; this file is the cell. The default is the live
//! `Close ⌘W` capture (`tab-bar-item.tsx`'s close button, focused), the one
//! this item's `/tmp/p3-ref-tooltip.json` is measured from.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real.** `content: ""` collapses the root to its padding and border around nothing — the box shrinks from `99.296875 × 32` to a fixed floor. It is the one flag this surface models, the same role it plays on `popover`. |
//! | `hover`, `focus`, `selected` | **unmodelled.** `tooltipContentBase` carries no interaction rule of its own — `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' tooltip.tsx` is empty, and the *reachability* mechanism (`element.focus()` on the trigger) is not a style this surface's own content class list carries. |
//! | `loading`, `error` | unmodelled, as on every surface. |
//!
//! Four of the six are declared on [`SURFACE`], which is what makes the
//! binary say so on stderr rather than rendering a cell that cannot fail.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::tooltip::{self, Tooltip};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "tooltip",
    root: tooltip::ID_TOOLTIP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // 32px content plus the caption below it, with headroom — a floor, not a
    // ceiling: this surface drives no height of its own.
    min_window_height: 90,
    // A popup floated over the page, whose width is its own content's.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--text`'s default: the live `Close ⌘W` fixture's text.
const DEFAULT_CONTENT: &str = "Close";

/// `--shortcut`'s default.
const DEFAULT_SHORTCUT: &str = "⌘W";

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--text`: the tooltip's text.
    pub content: SharedString,
    /// `--shortcut`: renders the keyboard-shortcut chip. `Some` by default —
    /// the reachable `Close ⌘W` fixture has one.
    pub shortcut: Option<SharedString>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            content: SharedString::new_static(DEFAULT_CONTENT),
            shortcut: Some(SharedString::new_static(DEFAULT_SHORTCUT)),
        }
    }
}

impl Params {
    /// The tooltip this cell describes.
    ///
    /// `empty` empties the content — a real, if unreached, picture: no live
    /// `TooltipContent` renders `""`, exactly as no live `PopoverContent`
    /// renders no children.
    #[must_use]
    pub fn tooltip(&self, cell: &Cell) -> Tooltip {
        Tooltip {
            content: if cell.has(StateFlag::Empty) {
                SharedString::new_static("")
            } else {
                self.content.clone()
            },
            shortcut: self.shortcut.clone(),
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
            "--text" => self.content = SharedString::from(value(args, option)?),
            "--shortcut" => self.shortcut = Some(SharedString::from(value(args, option)?)),
            "--no-shortcut" => self.shortcut = None,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The box's height is its own content's — no option here sets
    /// one, the same answer `checkbox`'s `size-*` gives.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let tooltip = self.tooltip(cell);
        let _ = write!(out, " · content {:?}", tooltip.content.as_ref());
        if let Some(shortcut) = &tooltip.shortcut {
            let _ = write!(out, " · shortcut {shortcut:?}");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no live TooltipContent renders \"\", so this cell has no reference",
            );
        }
    }

    /// The tooltip, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container, and a
    /// content-sized box drawn straight into one would be a block-level flex
    /// box — stretched to the container's width rather than hugging its own
    /// content. Every live `TooltipContent` is portalled and unconstrained;
    /// `kbd.rs`'s exact fix, applied here. The row carries no anchor, so it
    /// cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .child(self.tooltip(cell).render(theme, anchors))
            .into_any_element()
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--text <text>",
            format!("the tooltip's text [{DEFAULT_CONTENT:?}]"),
        ),
        (
            "--shortcut <text>",
            format!("the keyboard-shortcut chip [{DEFAULT_SHORTCUT:?}]"),
        ),
        (
            "--no-shortcut",
            "omit the chip — the path-breadcrumb fixture's shape".into(),
        ),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_CONTENT, DEFAULT_SHORTCUT, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "tooltip"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a tooltip cell carries this surface's bag")
    }

    /// The defaults are the live `Close ⌘W` fixture.
    #[test]
    fn the_defaults_are_the_live_close_shortcut_w_fixture() {
        let bag = Params::default();
        assert_eq!(bag.content.as_ref(), DEFAULT_CONTENT);
        assert_eq!(bag.shortcut.as_deref(), Some(DEFAULT_SHORTCUT));

        let tooltip = params_of(&cell(&[])).tooltip(&cell(&[]));
        assert_eq!(tooltip.content.as_ref(), "Close");
        assert_eq!(tooltip.shortcut.as_deref(), Some("⌘W"));
    }

    /// `--no-shortcut` reaches the path-breadcrumb shape.
    #[test]
    fn no_shortcut_reaches_the_breadcrumb_shape() {
        let no_shortcut = cell(&["--text", "demo/src/a/a.ts", "--no-shortcut"]);
        let tooltip = params_of(&no_shortcut).tooltip(&no_shortcut);
        assert_eq!(tooltip.content.as_ref(), "demo/src/a/a.ts");
        assert_eq!(tooltip.shortcut, None);
    }

    /// **`empty` is the one flag this surface models**, and it empties the
    /// content regardless of what `--text` said.
    #[test]
    fn empty_is_the_one_flag_that_moves_the_content() {
        let empty_cell = cell(&["--flags", "empty", "--text", "Close"]);
        let tooltip = params_of(&empty_cell).tooltip(&empty_cell);
        assert_eq!(tooltip.content.as_ref(), "");
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other four are declared unmodelled.
    #[test]
    fn the_four_interaction_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }

    /// The caption names the one cell with no reference.
    #[test]
    fn the_caption_names_the_cell_with_no_reference() {
        assert!(
            cell(&["--flags", "empty"])
                .describe()
                .contains("no reference")
        );
        assert!(!cell(&[]).describe().contains("no reference"));
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--text", "--shortcut", "--no-shortcut"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("tooltip"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// This surface's options belong to it and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--text", "--shortcut", "--no-shortcut"] {
            let line = ["--surface", "checkbox", option, "x"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a checkbox option",
            );
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "tooltip");
        assert_eq!(SURFACE.root, "tooltip");
        assert!(!SURFACE.full_bleed);
    }
}
