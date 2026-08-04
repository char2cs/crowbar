//! `--surface dropdown` — the open shell of a hand-rolled floating panel,
//! wrapped through `gpui_component::popover::Popover` the same way `popover`
//! (P3.15) is.
//!
//! `crowbar_ui::primitives::dropdown` carries the full division of labour and
//! the evidence that this is a distinct primitive from `dropdown-menu`; this
//! file is the cell.
//!
//! # The two options, and why both are declared rather than computed
//!
//! `--shell-width` and `--body-height` are runtime quantities this primitive
//! does not decide, for the same reason `popover`'s `--popup-width` and
//! `--body-height` are:
//!
//! * `min-w-[240px] max-w-[min(480px,…)]` is a CSS range with no `width` of
//!   its own; every live call site's `Dropdown` measures its own content once
//!   open and **locks** it onto the element as an inline `style.width`
//!   (`dropdown.tsx`'s `applyLockedWidth`) before anything settles. By the
//!   time a capture reads it, the used width is an explicit pixel value.
//! * the **body** is whatever a call site put inside the `role="menu"` div —
//!   5 of 7 live renders pass `children` outright, and the 2 that pass
//!   `items` render a different `MenuItem[]` each time. None of it is this
//!   primitive; see `crowbar_ui::primitives::dropdown`'s module docs in full.
//!
//! **Not spelled `--width`.** That flag is already the driver's own — the
//! surface's content-area width, `Cell::width_px` — and reusing it here would
//! be exactly the trap this item's own brief warns about: `--width` is the
//! *surface* width, `--viewport-width` is the *window*, and confusing either
//! with a third, primitive-owned quantity produces a constant delta that is
//! hard to trace back to a name collision. Caught by this surface's own test
//! before it shipped: an early draft named this option `--width` and every
//! cell silently rendered at the *global* default instead of the value the
//! test passed, because the shared parser claims `--width` before a
//! surface's own `accept` ever sees it.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover` | **unmodelled.** `grep -o 'hover:[a-z-]*' dropdown.tsx` finds one rule, `hover:bg-muted` on `dropdownItemVariants`'s `false` (not-disabled) arm — but that lives on `MenuItemsList`'s per-item rows, which this surface does not render (see the module docs' "body is a height" account). The shell itself carries no `hover:` rule at all. |
//! | `focus` | **unmodelled**, same reason: `focused` is a `dropdownItemVariants` axis, not a shell one. |
//! | `selected` | **unmodelled.** No row in `dropdown.tsx` carries a selected/checked state at all — unlike `dropdown-menu.tsx`'s `menu-checkbox-item`/`menu-radio-item`, this component has no such concept. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//! | `empty` | **real**, and the one flag this surface can fail on: a childless body collapses to `0`, so the shell's own height moves — the same shape `popover`'s `empty` flag is. |
//!
//! # The frame this surface needs
//!
//! Same mechanism as `popover`'s: `gpui_component::Popover` renders its popup
//! only from the frame *after* the trigger's `on_prepaint` has captured its
//! bounds, and the shared `row_layout` harness (`crowbar_driver::on_settled_frame`)
//! delivers it — see `native/mapping/popover.md` §4 for the full account, which
//! this surface leans on rather than repeats.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::dropdown::{self, Dropdown};
use gpui::{AnyElement, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "dropdown",
    root: dropdown::ID_ROOT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The shell's own chrome is two borders and two 4px paddings; the caption
    // sits below whatever `--body-height` drives. A floor, not a ceiling.
    min_window_height: 200,
    // A popup floated over the page (`fixed`), whose width is its own rather
    // than the viewport's.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--shell-width`'s default: `ceil(201.40625)`, the live
/// `file-explorer-tree.tsx` filter menu at rest. `crowbar_ui::primitives::dropdown`'s
/// module docs carry the measurement and why it is `content_sized` rather
/// than the locked pixel value `applyLockedWidth` writes.
pub const DEFAULT_WIDTH: u16 = dropdown::DEFAULT_WIDTH;

/// `--body-height`'s default: the same live filter menu's body — three
/// `dropdownItemVariants` rows and one separator, `30 + 30 + 5 + 30 = 95`.
pub const DEFAULT_BODY_HEIGHT: u16 = dropdown::DEFAULT_BODY_HEIGHT;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--shell-width`: the resolved, locked width `applyLockedWidth` leaves
    /// the element at.
    pub width: u16,
    /// `--body-height`: how tall a call site's content comes out.
    pub body_height: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            width: DEFAULT_WIDTH,
            body_height: DEFAULT_BODY_HEIGHT,
        }
    }
}

impl Params {
    /// The shell this cell describes.
    ///
    /// `empty` collapses the body to `0` — a menu that opens onto nothing is a
    /// real, reachable picture (`provider-switch-dropdown.tsx`'s own
    /// no-other-provider branch renders exactly this shape, a short message in
    /// place of any row).
    #[must_use]
    pub fn dropdown(&self, cell: &Cell) -> Dropdown {
        let empty = cell.has(StateFlag::Empty);
        Dropdown {
            width: px(f32::from(self.width)),
            body_height: if empty {
                px(0.0)
            } else {
                px(f32::from(self.body_height))
            },
        }
    }

    /// The shell's own height: two borders, two `p-1` paddings, the body.
    ///
    /// Spelled here rather than measured because
    /// [`SurfaceParams::driven_height`] is asked before a window exists.
    #[must_use]
    pub fn shell_height(&self, cell: &Cell) -> u16 {
        let dropdown = self.dropdown(cell);
        let chrome = f32::from(dropdown::BORDER_WIDTH) * 2.0 + f32::from(dropdown::PADDING) * 2.0;
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole number of px; \
                      `Cell` needs a `u16` to stay `Eq`"
        )]
        {
            (chrome + f32::from(dropdown.body_height)).ceil() as u16
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
            "--shell-width" => self.width = pixels(&value(args, option)?, option)?,
            "--body-height" => self.body_height = pixels(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **The shell's own height**, which `--body-height` decides, so the
    /// window follows it rather than capping it — the same reasoning
    /// `popover::Params::driven_height` gives.
    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.shell_height(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · shell-width {}px · body {}px",
            self.width, self.body_height,
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: the shell with a zero-height body");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.dropdown(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--shell-width <px>",
            format!("the shell's resolved, locked width — not `--width`, see the module docs [{DEFAULT_WIDTH}]"),
        ),
        (
            "--body-height <px>",
            format!("how tall a call site's content is [{DEFAULT_BODY_HEIGHT}]"),
        ),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_BODY_HEIGHT, DEFAULT_WIDTH, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "dropdown"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a dropdown cell carries this surface's bag")
    }

    /// The defaults are the live `file-explorer-tree.tsx` filter menu:
    /// `ceil(201.40625)` wide, three rows and a separator tall.
    #[test]
    fn the_defaults_are_the_live_filter_menu() {
        let bag = Params::default();
        assert_eq!(bag.width, DEFAULT_WIDTH);
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);

        let dropdown = bag.dropdown(&cell(&[]));
        assert_eq!(dropdown.width, px(202.0));
        assert_eq!(dropdown.body_height, px(95.0));
        // 1 + 4 + 95 + 4 + 1 — `/tmp/p3-ref-dropdown.json`'s own `h: 105`.
        assert_eq!(bag.shell_height(&cell(&[])), 105);
    }

    /// **`--shell-width` actually reaches [`Params::width`]** — the
    /// regression for the name-collision trap this surface's own module docs
    /// record: an earlier draft spelled this `--width` and every cell
    /// silently rendered at the default because the shared parser claims that
    /// name first.
    #[test]
    fn shell_width_reaches_the_params_bag_not_the_shared_width_flag() {
        for width in [200u16, 240, 400] {
            let driven = cell(&["--shell-width", &width.to_string()]);
            assert_eq!(params_of(&driven).width, width);
            assert_eq!(
                params_of(&driven).dropdown(&driven).width,
                px(f32::from(width))
            );
        }
    }

    /// **`empty` is the one flag this surface models**, and it collapses the
    /// body to a real, reachable zero — `provider-switch-dropdown.tsx`'s own
    /// no-other-provider branch is exactly this shape.
    #[test]
    fn empty_collapses_the_body_and_moves_the_shell_height() {
        let empty_cell = cell(&["--flags", "empty"]);
        let dropdown = params_of(&empty_cell).dropdown(&empty_cell);
        assert_eq!(dropdown.body_height, px(0.0));
        // 1 + 4 + 0 + 4 + 1.
        assert_eq!(params_of(&empty_cell).shell_height(&empty_cell), 10);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other five are declared unmodelled.
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

    /// `--body-height` moves the shell's height, and the window follows it.
    #[test]
    fn the_body_height_drives_the_shell_and_the_window() {
        let tall = cell(&["--body-height", "400"]);
        assert_eq!(params_of(&tall).shell_height(&tall), 410);
        assert!(tall.window_extent() > 410);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--shell-width", "wide"],
            vec!["--shell-width"],
            vec!["--body-height", "-1"],
            vec!["--body-height"],
            vec!["--variant", "ghost"],
        ] {
            let mut full = vec!["--surface", "dropdown"];
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

    /// These options belong to this surface and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--shell-width", "--body-height"] {
            let line = ["--surface", "git-status-row", option, "24"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--shell-width", "--body-height"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("dropdown"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The caption names the one cell that has no reference to compare
    /// against, once that cell exists — currently every default-shaped cell
    /// this surface can render has a live call site.
    #[test]
    fn the_caption_names_the_empty_cell() {
        assert!(
            cell(&["--flags", "empty"])
                .describe()
                .contains("zero-height body")
        );
        assert!(!cell(&[]).describe().contains("zero-height"));
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "dropdown");
        assert_eq!(SURFACE.root, "dropdown-root");
    }
}
