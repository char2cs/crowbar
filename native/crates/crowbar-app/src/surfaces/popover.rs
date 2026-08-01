//! `--surface popover` — the first surface drawn by a **wrapped**
//! `gpui-component` widget rather than by this design system's own `div()`s.
//!
//! An **open** popover popup. `crowbar_ui::components::popover` carries the
//! division of labour and what had to be overridden to reach it; this file is
//! the cell.
//!
//! # The two options that are not matrix axes
//!
//! `--popup-width` and `--body-height` are both runtime quantities this
//! primitive does not decide, and they are parameters for the same reason
//! `dropdown-menu`'s `--anchor-width` is:
//!
//! * `w-(--popup-width,auto)` is written onto the popup by Floating UI, and
//!   every live call site replaces it from its own `className` —
//!   `repo-icon-popover`'s is `w-64 p-0`, so 256.
//! * the **body** is `PopoverContent`'s children, which are a different sub-UI
//!   at every call site. The port takes their measured extent rather than
//!   reproducing one of them, so that the two boxes that genuinely *are* this
//!   primitive — the popup and its viewport — compare against a reference whose
//!   content is whatever it happens to be.
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `hover`, `focus`, `selected` | **unmodelled.** `popover.tsx` has no `hover:`, no `focus:` and no selected rule anywhere in either class list — the only interaction styling is `data-starting-style:` (mount) and `data-instant:` (transition suppression), neither of which is a §8.3 flag. `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' popover.tsx` is empty. |
//! | `empty` | **real.** `PopoverContent` with no children is a picture this primitive genuinely has: the viewport collapses to its own padding and the popup to that plus its two borders, so **both** anchors move — 254×175 → 254×32, 256×177 → 256×34. It is the one flag on this surface whose cell can fail. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//!
//! Five of the six are therefore declared on [`SURFACE`], which is what makes
//! the binary say so on stderr instead of rendering a cell that cannot fail.
//!
//! `empty` is **modelled but unreached**: no live call site renders a childless
//! popover, so the cell has no reference — which is a different thing from
//! having no state, and the caption says which.
//!
//! # The frame this surface needs, and where it now comes from
//!
//! `gpui_component::Popover` renders its popup **only after** the trigger's
//! bounds have been captured — `if !open || !trigger_bounds_captured { return
//! el; }` — and that capture happens in the trigger's own `on_prepaint`, which
//! is *after* `RenderOnce::render` has already returned. So frame 1 contains the
//! trigger and no popup; the vendor calls `window.request_animation_frame()` and
//! frame 2 contains both.
//!
//! When this surface was written, both consumers gave it one frame. `main.rs`
//! emitted from `window.on_next_frame`, which gpui runs at the **top** of a
//! frame request, before `window.draw`:
//!
//! ```text
//! $ CROWBAR_ROW_SNAPSHOT=… crowbar-app --surface popover
//! crowbar-app: no snapshot: the root anchor "popover-popup" was not recorded
//!              this frame; the anchors that were: []
//! ```
//!
//! P3.17 replaced the frame *index* with a signal: the capture is taken on the
//! first completed draw that reproduced the previous completed draw's anchors —
//! `crowbar_driver::on_settled_frame`, documented in `crowbar-driver`'s
//! `src/frame.rs`. The same signal drives the shared `row_layout` harness, which
//! is why `row_layout/popover.rs` no longer carries a local copy of `measure_in`
//! and why this surface now emits:
//!
//! ```text
//! $ CROWBAR_ROW_SNAPSHOT=… crowbar-app --surface popover
//! crowbar-app: snapshot written to …   # popover-popup 256 × 177
//! ```
//!
//! `native/mapping/popover.md` §4 carries the mechanism and the generalisation
//! to the rest of the §6.2 list, every one of which needed the same frame.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::popover::{self, Popover, Variant};
use gpui::{AnyElement, SharedString, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "popover",
    root: popover::ID_POPUP,
    // Every §8.3 flag but `empty`: `popover.tsx` carries no interaction rule at
    // all. See the module docs' table for the grep, and for why `empty` is the
    // one that is real.
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live popup is 177px; the caption sits below whatever `--body-height`
    // drives. A floor, not a ceiling — `driven_height` moves the window.
    min_window_height: 240,
    // A popup floated over the page by Floating UI, whose width is its own
    // rather than the viewport's.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--popup-width`'s default: `repo-icon-popover`'s own `className="w-64"`,
/// which is the only popover a parity run can currently reach.
pub const DEFAULT_POPUP_WIDTH: u16 = 256;

/// `--body-height`'s default: the same call site's children, measured off the
/// running app at 143px — its 175px viewport less 16px of padding either side.
pub const DEFAULT_BODY_HEIGHT: u16 = 143;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--popup-width`: `w-(--popup-width,auto)`, or a call site's own `w-*`.
    pub popup_width: u16,
    /// `--body-height`: how tall `PopoverContent`'s children come out.
    pub body_height: u16,
    /// `--title`: renders a `PopoverTitle` above the body.
    ///
    /// `None` by default, and that is not a convenience: the one popover a
    /// parity run can drive renders no title, and an anchor the reference
    /// cannot produce is a `FieldPresence` delta that forgives nothing.
    pub title: Option<SharedString>,
    /// `--tooltip`: the `tooltipStyle` appearance.
    pub tooltip: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            popup_width: DEFAULT_POPUP_WIDTH,
            body_height: DEFAULT_BODY_HEIGHT,
            title: None,
            tooltip: false,
        }
    }
}

impl Params {
    /// The variant this cell selects.
    #[must_use]
    pub const fn variant(&self) -> Variant {
        if self.tooltip {
            Variant::Tooltip
        } else {
            Variant::Default
        }
    }

    /// The popup this cell describes.
    ///
    /// `empty` empties it: no children, and therefore no title either — a
    /// `PopoverContent` with a heading and nothing under it is not what the flag
    /// means, and modelling it that way would leave one anchor unmoved.
    #[must_use]
    pub fn popover(&self, cell: &Cell) -> Popover {
        let empty = cell.has(StateFlag::Empty);
        Popover {
            width: px(f32::from(self.popup_width)),
            body_height: if empty {
                px(0.0)
            } else {
                px(f32::from(self.body_height))
            },
            variant: self.variant(),
            title: if empty { None } else { self.title.clone() },
        }
    }

    /// The popup's own height: two borders, two paddings, the title where there
    /// is one, and the body.
    ///
    /// Spelled here rather than measured because [`SurfaceParams::driven_height`]
    /// is asked *before* a window exists — the window's size is what it decides.
    #[must_use]
    pub fn popup_height(&self, cell: &Cell) -> u16 {
        let popover = self.popover(cell);
        let chrome = f32::from(popover::BORDER_WIDTH) * 2.0
            + f32::from(popover.variant.viewport_padding_y()) * 2.0;
        let title = if popover.title.is_some() {
            TITLE_HEIGHT
        } else {
            0.0
        };
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole number of px; \
                      `Cell` needs a `u16` to stay `Eq`"
        )]
        {
            (chrome + title + f32::from(popover.body_height)).ceil() as u16
        }
    }
}

/// `text-lg leading-none` — an 18px line box, and the title's whole border box.
const TITLE_HEIGHT: f32 = 18.0;

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--popup-width" => self.popup_width = pixels(&value(args, option)?, option)?,
            "--body-height" => self.body_height = pixels(&value(args, option)?, option)?,
            "--title" => self.title = Some(SharedString::from(value(args, option)?)),
            "--tooltip" => self.tooltip = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **The popup's own height**, which `--body-height` decides — so the
    /// window follows it rather than capping it.
    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.popup_height(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · popup {}px · body {}px",
            self.popup_width, self.body_height,
        );
        if self.tooltip {
            out.push_str(" · tooltipStyle: no live call site, so this cell has no reference");
        }
        if self.title.is_some() {
            out.push_str(" · title: the reachable popover has none, so this cell has no reference");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no call site renders a childless popover, so this cell has no reference",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.popover(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--popup-width <px>",
            format!("w-(--popup-width): the call site's own width [{DEFAULT_POPUP_WIDTH}]"),
        ),
        (
            "--body-height <px>",
            format!("how tall PopoverContent's children are [{DEFAULT_BODY_HEIGHT}]"),
        ),
        (
            "--title <text>",
            "render a PopoverTitle; no live popover a run can reach has one".into(),
        ),
        (
            "--tooltip",
            "tooltipStyle: rounded-md, text-xs, a py-1/px-2 viewport".into(),
        ),
    ]
    .into_iter()
    .map(|(option, description)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_BODY_HEIGHT, DEFAULT_POPUP_WIDTH, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::popover::Variant;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "popover"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a popover cell carries this surface's bag")
    }

    /// The defaults are the live `repo-icon-popover`, measured off the running
    /// app: a 256px popup whose 177px height is 1 + 16 + 143 + 16 + 1.
    #[test]
    fn the_defaults_are_the_live_repo_icon_popover() {
        let bag = Params::default();

        assert_eq!(bag.popup_width, DEFAULT_POPUP_WIDTH);
        assert_eq!(bag.body_height, DEFAULT_BODY_HEIGHT);
        assert_eq!(bag.title, None);
        assert!(!bag.tooltip);
        assert_eq!(bag.variant(), Variant::Default);
        assert_eq!(bag.popup_height(&cell(&[])), 177);

        let popover = bag.popover(&cell(&[]));
        assert_eq!(popover.width, px(256.0));
        assert_eq!(popover.body_height, px(143.0));
    }

    /// **`empty` is the only flag this surface models**, and it moves both
    /// anchors: a childless popup is its padding and its borders and nothing
    /// else. The other five would compare resting against resting, because
    /// `popover.tsx` carries no interaction rule at all.
    #[test]
    fn empty_is_the_one_flag_that_moves_a_box() {
        let resting = params_of(&cell(&[])).popup_height(&cell(&[]));
        let empty_cell = cell(&["--flags", "empty"]);
        let empty = params_of(&empty_cell).popup_height(&empty_cell);

        // 1 + 16 + 0 + 16 + 1, against 1 + 16 + 143 + 16 + 1.
        assert_eq!(empty, 34);
        assert_eq!(resting, 177);
        assert!(!SURFACE.unmodelled(StateFlag::Empty));

        // And it empties the title too: a heading with nothing under it is not
        // what the flag means.
        let titled = cell(&["--flags", "empty", "--title", "Commit changes"]);
        assert_eq!(params_of(&titled).popover(&titled).title, None);
        assert_eq!(params_of(&titled).popup_height(&titled), 34);
    }

    /// The other five are declared unmodelled, which is what makes the binary
    /// say so on stderr rather than drawing a cell that cannot fail.
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

    /// `--body-height` moves the popup's height, and the window follows it —
    /// which is what stops a tall popup being cut by a window sized for a short
    /// one.
    #[test]
    fn the_body_height_drives_the_popup_and_the_window() {
        let tall = cell(&["--body-height", "400"]);
        assert_eq!(params_of(&tall).popup_height(&tall), 434);
        assert!(tall.window_extent() > 434);

        // The `tooltipStyle` viewport is `py-1`, so the same body makes a
        // shorter popup: 1 + 4 + 400 + 4 + 1.
        let tooltip = cell(&["--body-height", "400", "--tooltip"]);
        assert_eq!(params_of(&tooltip).popup_height(&tooltip), 410);

        // A title adds exactly its own 18px line box.
        let titled = cell(&["--title", "Merge into main"]);
        assert_eq!(params_of(&titled).popup_height(&titled), 177 + 18);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--popup-width", "wide"],
            vec!["--popup-width"],
            vec!["--body-height", "-1"],
            vec!["--body-height"],
            vec!["--title"],
            vec!["--variant", "tooltip"],
        ] {
            let mut full = vec!["--surface", "popover"];
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

    /// **These options belong to this surface and to no other**, which is the
    /// property the registry exists for.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--popup-width", "--body-height", "--tooltip"] {
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

        for option in ["--popup-width", "--body-height", "--title", "--tooltip"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("popover"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The caption says when a cell has **no reference to compare against**,
    /// which is per-cell where `Surface::unmodelled` is per-surface.
    #[test]
    fn the_caption_names_the_two_cells_that_have_no_reference() {
        assert!(cell(&["--tooltip"]).describe().contains("no reference"));
        assert!(
            cell(&["--flags", "empty"])
                .describe()
                .contains("no reference")
        );
        assert!(
            cell(&["--title", "Commit changes"])
                .describe()
                .contains("no reference")
        );
        // And says nothing of the sort at the reachable default.
        assert!(!cell(&[]).describe().contains("no reference"));
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "popover");
        assert_eq!(SURFACE.root, "popover-popup");
    }
}
