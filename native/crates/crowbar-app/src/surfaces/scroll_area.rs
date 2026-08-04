//! `--surface scroll-area` — a scroll viewport, and the surface where
//! **`--width` is finally real**.
//!
//! `crowbar_ui::primitives::scroll_area` carries the wrap-versus-build seam
//! evidence and what the vendor could not supply; this file is the cell.
//!
//! # `--width` drives the root, because `size-full` means it does
//!
//! Most leaf surfaces declare `--width` vacuous: nothing on them is a
//! percentage. This one is the opposite case. `ScrollAreaPrimitive.Root` is
//! `size-full min-h-0` and authors **no width of its own at all** — the live
//! `workspace-tree` instance is 344px because the sidebar panel it sits in is,
//! and the command palette's is 574 for the same reason. So the surface column
//! *is* the containing block, and `--width` moves both anchors together.
//!
//! `--area-height` is the other half, and it is a parameter rather than an axis
//! because the surface column has no height to inherit. It is the same call
//! `popover` makes about `--body-height`: a runtime quantity the primitive does
//! not decide, declared rather than computed.
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `hover` | **real on the scrollbar, invisible on both anchors.** `data-hovering:opacity-100` is the only interaction rule in either class list and it is on the *track*, which carries no anchor and is not mounted in any reachable cell. Declared unmodelled: a cell that cannot move a compared field is a cell that cannot fail. |
//! | `focus` | **unmodelled.** `focus-visible:ring-2` is on the viewport, and `document.hasFocus()` is false and immovable in the harness — the reference can never produce the ring. |
//! | `selected`, `loading`, `error` | unmodelled, as on every surface so far. |
//! | `empty` | **modelled**, and it is the one flag this surface genuinely has. |
//!
//! Five are therefore declared, which is what makes the binary say so on stderr
//! instead of rendering a cell that cannot fail.
//!
//! # `empty` is real, and what it moves is the **picture** rather than the snapshot
//!
//! Both halves of that are worth stating, because half of it on its own would
//! mislead.
//!
//! It is **real**: `{children}` is a prop, rendering none is an expressible
//! picture, and base-ui *reacts* to it — overflow is computed from content, so
//! a childless viewport can never mount a scrollbar or a corner. The cell
//! therefore forces [`Overflow::NONE`] however `--overflow` was spelled, which
//! is the same coupling `popover`'s `empty` makes when it drops the title along
//! with the body.
//!
//! And **neither anchored box moves**, because neither is sized by its content:
//! the root is `size-full` inside the call site's layout and the viewport is
//! `h-full` inside the root. So an `empty` cell compares equal on every field
//! the contract carries.
//!
//! That is *not* the same fact `unmodelled` announces. `unmodelled` means the
//! React original has **no such state to disagree with** — `popover.tsx` has no
//! `hover:` rule anywhere, `kbd.tsx` carries `pointer-events-none`. This one has
//! the state; what it lacks is a box whose size depends on it. The caption says
//! so per cell rather than leaving a reader to infer coverage from a flag that
//! was accepted.
//!
//! # `--overflow` is the one option that is **not** a matrix axis
//!
//! It decides whether base-ui mounts a scrollbar, and it is modelled because the
//! component genuinely has the branch — but **no cell of it can be compared**.
//! `ANCHORS.md` v1.8 keeps the declared anchor set to the two boxes that are
//! always there; the tracks it mounts carry no anchor by construction. Driving
//! `--overflow both` changes the picture and leaves the snapshot identical,
//! which the caption says out loud.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::scroll_area::{self, Overflow, ScrollArea};
use gpui::{AnyElement, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "scroll-area",
    root: scroll_area::ID_ROOT,
    // Every §8.3 flag but `empty`. See the module docs' table: the one
    // interaction rule in either class list is `data-hovering:opacity-100` on
    // the unanchored track, and `focus-visible:ring-2` is a ring the reference
    // can never produce — `document.hasFocus()` is false and immovable.
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live area is 936px tall; the caption sits below whatever
    // `--area-height` drives. A floor, not a ceiling — `driven_height` moves the
    // window.
    min_window_height: 240,
    // A `ScrollArea` fills whatever pane holds it, and every live one is a
    // sidebar panel rather than the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--area-height`'s default: the live `workspace-tree` area, measured off the
/// running app at 936px.
pub const DEFAULT_AREA_HEIGHT: u16 = 936;

/// The width the reference was captured at, and the default `--width` this
/// surface wants.
///
/// Not a `Params` field — `--width` is the shared axis and this is what to pass
/// it. Named so the tests and the usage cannot drift from the reference.
pub const REFERENCE_WIDTH: u16 = 344;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--area-height`: how tall the call site's layout makes the root.
    pub area_height: u16,
    /// `--overflow`: which axes overflow, and therefore which scrollbars
    /// base-ui mounts. **Moves no compared field** — see the module docs.
    pub overflow: Overflow,
    /// `--fade`: the `scrollFade` prop's four edge masks. `ANCHORS.md` §6 — no
    /// field, either side.
    pub fade: bool,
    /// `--gutter`: the `scrollbarGutter` prop's padding on an overflowing axis.
    pub gutter: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            area_height: DEFAULT_AREA_HEIGHT,
            overflow: Overflow::NONE,
            fade: false,
            gutter: false,
        }
    }
}

impl Params {
    /// The area this cell describes.
    ///
    /// `empty` forces [`Overflow::NONE`]: base-ui computes overflow from
    /// content, so a childless viewport can never mount a track whatever
    /// `--overflow` said. See the module docs.
    #[must_use]
    pub fn area(&self, cell: &Cell) -> ScrollArea {
        ScrollArea {
            width: cell.width_px(),
            height: px(f32::from(self.area_height)),
            overflow: if cell.has(StateFlag::Empty) {
                Overflow::NONE
            } else {
                self.overflow
            },
            fade: self.fade,
            gutter: self.gutter,
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
            "--area-height" => self.area_height = pixels(&value(args, option)?, option)?,
            "--overflow" => self.overflow = parse_overflow(&value(args, option)?)?,
            "--fade" => self.fade = true,
            "--gutter" => self.gutter = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **The root's own height**, which `--area-height` decides — so the window
    /// follows it rather than capping it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        Some(self.area_height)
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · area {}×{}px", cell.width, self.area_height);
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no children, so base-ui mounts no track — but neither anchor is \
                 sized by its content, so every compared field is the resting one",
            );
        }
        if self.overflow != Overflow::NONE && !cell.has(StateFlag::Empty) {
            out.push_str(
                " · overflow: mounts a scrollbar, which carries no anchor \
                 (ANCHORS.md v1.8 — its presence is a cell property), so the snapshot is unmoved",
            );
        }
        if self.fade {
            out.push_str(" · scrollFade: four edge masks, which ANCHORS.md §6 cannot see");
        }
        if self.gutter && self.overflow == Overflow::NONE {
            out.push_str(" · scrollbarGutter: inert without an overflowing axis");
        }
        if cell.width != REFERENCE_WIDTH {
            let _ = write!(
                out,
                " · the reference was captured at --width {REFERENCE_WIDTH}",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.area(cell).render(theme, anchors)
    }
}

/// `--overflow`'s closed vocabulary.
fn parse_overflow(raw: &str) -> Result<Overflow, ParseError> {
    match raw {
        "none" => Ok(Overflow::NONE),
        "y" => Ok(Overflow { y: true, x: false }),
        "x" => Ok(Overflow { y: false, x: true }),
        "both" => Ok(Overflow { y: true, x: true }),
        other => Err(ParseError::Rejected(format!(
            "--overflow takes none, y, x or both, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--area-height <px>".to_owned(),
            format!("how tall the call site's layout makes the root [{DEFAULT_AREA_HEIGHT}]"),
        ),
        (
            "--overflow <none|y|x|both>".to_owned(),
            "which axes overflow, and so which scrollbars base-ui mounts; \
             they carry no anchor, so the snapshot does not move [none]"
                .to_owned(),
        ),
        (
            "--fade".to_owned(),
            "scrollFade: four edge masks, which ANCHORS.md §6 cannot see".to_owned(),
        ),
        (
            "--gutter".to_owned(),
            "scrollbarGutter: 10px of viewport padding on an overflowing axis".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_AREA_HEIGHT, Params, REFERENCE_WIDTH, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::primitives::scroll_area::{GUTTER, Overflow};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "scroll-area"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a scroll-area cell carries this surface's bag")
    }

    /// The defaults are the live `workspace-tree` area, measured off the running
    /// app: `344 × 936` with neither axis overflowing.
    #[test]
    fn the_defaults_are_the_live_workspace_tree_area() {
        let bag = Params::default();

        assert_eq!(bag.area_height, DEFAULT_AREA_HEIGHT);
        assert_eq!(bag.overflow, Overflow::NONE);
        assert!(!bag.fade);
        assert!(!bag.gutter);

        let cell = cell(&["--width", "344"]);
        let area = params_of(&cell).area(&cell);
        assert_eq!(area.width, px(f32::from(REFERENCE_WIDTH)));
        assert_eq!(area.height, px(936.0));
    }

    /// **`--width` is real on this surface**, which almost no leaf can say: the
    /// root is `size-full`, so its width *is* the container's, and both anchors
    /// move together.
    #[test]
    fn the_shared_width_axis_drives_the_root() {
        for width in [200u16, 344, 574] {
            let cell = cell(&["--width", &width.to_string()]);
            assert_eq!(
                params_of(&cell).area(&cell).width,
                px(f32::from(width)),
                "--width {width}",
            );
        }
    }

    /// **Five of the six are declared unmodelled**, which is what makes the
    /// binary say so on stderr rather than drawing a cell that cannot fail.
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

    /// **`empty` is modelled, drops every track, and moves no compared field** —
    /// all three, because any one of them alone would misread.
    ///
    /// `unmodelled` means the original has no such state. This one has it:
    /// base-ui computes overflow from content, so a childless viewport cannot
    /// mount a scrollbar whatever `--overflow` said. What it lacks is a box
    /// whose size depends on it, and the caption is where that is stated.
    #[test]
    fn empty_drops_every_track_and_still_moves_no_anchored_box() {
        assert!(!SURFACE.unmodelled(StateFlag::Empty));

        let asked = cell(&["--width", "344", "--overflow", "both"]);
        let emptied = cell(&["--width", "344", "--overflow", "both", "--flags", "empty"]);

        // The flag overrides the option outright …
        assert_eq!(
            params_of(&asked).area(&asked).overflow,
            Overflow { y: true, x: true }
        );
        assert_eq!(params_of(&emptied).area(&emptied).overflow, Overflow::NONE);

        // … and the two anchored boxes are the same either way.
        let full = params_of(&asked).area(&asked);
        let bare = params_of(&emptied).area(&emptied);
        assert_eq!(full.width, bare.width);
        assert_eq!(full.height, bare.height);

        assert!(
            emptied
                .describe()
                .contains("every compared field is the resting one")
        );
    }

    /// `--area-height` moves the root, and the window follows it — which is what
    /// stops a tall area being cut by a window sized for a short one.
    #[test]
    fn the_area_height_drives_the_root_and_the_window() {
        let tall = cell(&["--area-height", "1200"]);
        assert_eq!(params_of(&tall).area(&tall).height, px(1200.0));
        assert!(tall.window_extent() > 1200);
    }

    /// **`--overflow` moves the picture and not the snapshot**, which is the
    /// distinction v1.8 draws and the caption states.
    #[test]
    fn the_overflow_option_changes_no_anchored_box() {
        let none = cell(&["--width", "344"]);
        let both = cell(&["--width", "344", "--overflow", "both"]);

        let plain = params_of(&none).area(&none);
        let scrolled = params_of(&both).area(&both);

        // The two anchors are identical …
        assert_eq!(plain.width, scrolled.width);
        assert_eq!(plain.height, scrolled.height);
        // … and the mounted tracks are not.
        assert_ne!(plain.overflow, scrolled.overflow);
        assert!(scrolled.overflow.has_corner());
        assert!(both.describe().contains("carries no anchor"));
    }

    /// The gutter needs the prop **and** an overflowing axis, and it is the one
    /// option pair that can move the viewport's padding.
    #[test]
    fn the_gutter_only_pads_an_overflowing_axis() {
        let inert = cell(&["--gutter"]);
        assert_eq!(params_of(&inert).area(&inert).gutter_end(), px(0.0));
        assert!(inert.describe().contains("inert"));

        let live = cell(&["--gutter", "--overflow", "y"]);
        assert_eq!(params_of(&live).area(&live).gutter_end(), GUTTER);
        assert_eq!(params_of(&live).area(&live).gutter_bottom(), px(0.0));
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--area-height", "tall"],
            vec!["--area-height"],
            vec!["--overflow", "vertical"],
            vec!["--overflow"],
            vec!["--fade", "yes", "--nonsense"],
            vec!["--scroll-fade"],
        ] {
            let mut full = vec!["--surface", "scroll-area"];
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
        for option in ["--area-height", "--overflow", "--fade", "--gutter"] {
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

        for option in ["--area-height", "--overflow", "--fade", "--gutter"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("scroll-area"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The caption names the width the reference was taken at, so a run driven
    /// somewhere else says so rather than comparing against nothing.
    #[test]
    fn the_caption_names_a_width_that_is_not_the_references() {
        assert!(
            !cell(&["--width", "344"])
                .describe()
                .contains("the reference was captured"),
        );
        assert!(
            cell(&["--width", "500"])
                .describe()
                .contains("the reference was captured at --width 344"),
        );
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "scroll-area");
        assert_eq!(SURFACE.root, "scroll-area-root");
    }
}
