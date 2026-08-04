//! `--surface sidebar-peek` — the hover-to-peek host, at one of its three
//! resting `data-state`s.
//!
//! `crowbar_ui::surfaces::sidebar::sidebar_peek` carries the full account of this
//! composition's own arithmetic, the no-anchor-on-the-outer-wrapper
//! argument and the re-derivation of the cluster's own transition-end-state
//! reasoning. This file is the cell.
//!
//! # `full_bleed`, and the `--width == --viewport-width` convention it needs
//!
//! [`PeekState::Closed`]/[`PeekState::Peeking`] are `position: absolute`
//! with no `.relative()` ancestor of their own — the identical shape
//! `fps_overlay.rs` documents in `surface.rs`'s own `full_bleed` field: taffy
//! resolves `position: absolute` against the **immediate parent**
//! unconditionally, so reaching the window's *true* edges needs that
//! immediate parent to be given the window's own uninset width, which is
//! exactly what [`Surface::full_bleed`] buys. Following `fps_overlay.rs`'s
//! own `row_layout` convention directly: a [`PeekState::Closed`]/
//! [`PeekState::Peeking`] cell should drive `--width` equal to
//! `--viewport-width`, or the card's `left`/`right` offset is measured off
//! a narrower box than the reference's own window.
//!
//! For the identical reason, [`Params::render`] wraps
//! [`PeekState::Docked`] in a height-driving column but hands
//! [`PeekState::Closed`]/[`PeekState::Peeking`] to
//! [`crowbar_ui::surfaces::sidebar::sidebar_peek::SidebarPeek::render`] with
//! **no** wrapping element at all: an intervening `div` with its own
//! authored height would become the absolute box's immediate parent
//! instead of the harness's own full-window box, and the card's `top`/
//! `bottom` insets would land against *that* box's edges, not the
//! window's.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `sidebar-peek.tsx`'s own two `<div>`s carry no `hover:`/`focus:`/`data-active` rule at all — every visual difference is `data-state`, this surface's own axis, not the §8.3 vocabulary. `--state`, `--right`, `--peek-width`, `--content-width` and `--height` are this surface's own options instead. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::sidebar::sidebar_peek::{self, PeekState, SidebarPeek};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-peek",
    root: sidebar_peek::ID_ROOT,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The same floor `sidebar-carousel`/`nav-stack` take: this component
    // wraps the *entire* sidebar column in the real app, so a sidebar-sized
    // column is the honest default.
    min_window_height: 700,
    // See the module docs: needed for `Closed`/`Peeking`'s own absolute
    // positioning to reach the window's true edges, and harmless for
    // `Docked` — its own box is `w_full()`/`h_full()`, unaffected by the
    // ambient inset either way.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// `--height`'s default, in logical px — the same number `sidebar-carousel`
/// and `nav-stack` take.
pub const DEFAULT_HEIGHT: u16 = 600;

/// `--peek-width`'s default, in logical px — a plausible remembered sidebar
/// width, matching `sidebar-carousel`'s own live-measured `SURFACE_WIDTH`.
pub const DEFAULT_PEEK_WIDTH: u16 = 294;

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--state`: `data-state`.
    pub state: PeekState,
    /// `--right`: `side === 'right'`.
    pub right: bool,
    /// `--peek-width`: `width` — read in every state, though only painted
    /// explicitly outside `Docked`. See the component's own module docs.
    pub peek_width: u16,
    /// `--content-width`: drives the claim that the card's box does not
    /// depend on the opaque sidebar column it wraps.
    pub content_width: u16,
    /// `--height`: the column `Docked`'s `h_full()` resolves against, and
    /// the window height `Closed`/`Peeking`'s own `inset-y-2` resolves
    /// against.
    pub height: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            state: PeekState::Docked,
            right: false,
            peek_width: DEFAULT_PEEK_WIDTH,
            content_width: 0,
            height: DEFAULT_HEIGHT,
        }
    }
}

impl Params {
    /// The card this cell describes.
    ///
    /// `content_height` is **not** `self.height`, and **not** bare
    /// [`Cell::window_extent`] either — both were tried and measured wrong.
    /// `self.height` under-measures once the surface's own floor takes over
    /// (`min_window_height` is 700 against a 600 default `--height`, so the
    /// real window is taller than `self.height` says). Bare
    /// `window_extent` measures the card 16px short at the bottom —
    /// confirmed by running the `row_layout` test with the card's raw bounds
    /// printed, not derived on paper — because `window_extent` is the room
    /// *below* `RowSurface`'s own unconditional `pt(INSET_Y)`, and this
    /// card's `top` offset is measured from that same already-inset edge
    /// while its `bottom` is meant to reach the window's **true**, un-inset
    /// bottom. `INSET_Y + window_extent` is `window.height - INSET_Y`
    /// exactly — the room from the harness's inset top edge down to the
    /// window's real bottom — and it is what makes the card's own bottom
    /// edge land exactly `PEEK_MARGIN` off the window's true bottom, the
    /// same gap its top edge would have if the harness itself did not
    /// artificially inset every surface's top by `INSET_Y` regardless of
    /// `full_bleed`. See `row_layout::sidebar_peek`'s own doc comments for
    /// the arithmetic in full and the two wrong values it replaced.
    #[must_use]
    pub fn peek(&self, cell: &Cell) -> SidebarPeek {
        SidebarPeek {
            state: self.state,
            is_right: self.right,
            peek_width: px(f32::from(self.peek_width)),
            viewport_width: cell.viewport_width_px(),
            content_height: px(crate::row_surface::INSET_Y + f32::from(cell.window_extent())),
            content_width: px(f32::from(self.content_width)),
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
            "--state" => self.state = parse_state(&value(args, option)?)?,
            "--right" => self.right = true,
            "--peek-width" => self.peek_width = pixels(&value(args, option)?, option)?,
            "--content-width" => self.content_width = pixels(&value(args, option)?, option)?,
            "--height" => {
                let height = pixels(&value(args, option)?, option)?;
                if height == 0 {
                    return Err(ParseError::Rejected(
                        "--height must be greater than zero: a column with no height leaves \
                         the docked case's own h-full nothing to resolve against"
                            .to_owned(),
                    ));
                }
                self.height = height;
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        Some(self.height)
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · {} · {} · peek-width {}px · {}px tall",
            self.state.name(),
            if self.right { "right" } else { "left" },
            self.peek_width,
            self.height,
        );
        if self.content_width > 0 {
            let _ = write!(out, " · content {}px", self.content_width);
        }
        if !matches!(self.state, PeekState::Docked) {
            out.push_str(
                " · --width should equal --viewport-width, or this card's left/right offset \
                 is measured off a narrower box than the window",
            );
        }
    }

    /// **`true`, unconditionally.** See the module docs and this surface's
    /// own `unmodelled` list.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let peek = self.peek(cell);
        if matches!(self.state, PeekState::Docked) {
            // See the module docs: only `Docked` gets a wrapping column —
            // `Closed`/`Peeking` must reach the harness's own full-window
            // box directly.
            div()
                .flex()
                .flex_col()
                .h(px(f32::from(self.height)))
                .child(peek.render(theme, anchors))
                .into_any_element()
        } else {
            peek.render(theme, anchors)
        }
    }
}

/// `--state`'s closed vocabulary — `data-state`'s own three words.
fn parse_state(raw: &str) -> Result<PeekState, ParseError> {
    match raw {
        "docked" => Ok(PeekState::Docked),
        "closed" => Ok(PeekState::Closed),
        "peeking" => Ok(PeekState::Peeking),
        other => Err(ParseError::Rejected(format!(
            "--state takes docked, closed or peeking, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--state <docked|closed|peeking>".to_owned(),
            "data-state's own three words [docked]".to_owned(),
        ),
        (
            "--right".to_owned(),
            "side === 'right': docks/peeks off the right edge instead [left]".to_owned(),
        ),
        (
            "--peek-width <px>".to_owned(),
            format!("the remembered sidebar width [{DEFAULT_PEEK_WIDTH}]"),
        ),
        (
            "--content-width <px>".to_owned(),
            "filler inside the card; drives the claim that it ignores content [0]".to_owned(),
        ),
        (
            "--height <px>".to_owned(),
            format!(
                "the docked column's own height, and the window height closed/peeking \
                 resolve inset-y-2 against [{DEFAULT_HEIGHT}]"
            ),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_HEIGHT, DEFAULT_PEEK_WIDTH, Params, SURFACE, options, parse_state};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::surfaces::sidebar::sidebar_peek::PeekState;
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-peek"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sidebar-peek cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_docked_state() {
        let bag = Params::default();
        assert_eq!(bag.state, PeekState::Docked);
        assert!(!bag.right);
        assert_eq!(bag.peek_width, DEFAULT_PEEK_WIDTH);
        assert_eq!(bag.content_width, 0);
        assert_eq!(bag.height, DEFAULT_HEIGHT);

        let peek = params_of(&cell(&[])).peek(&cell(&[]));
        assert_eq!(peek.state, PeekState::Docked);
    }

    #[test]
    fn every_option_reaches_the_card_independently() {
        let driven = cell(&[
            "--state",
            "peeking",
            "--right",
            "--peek-width",
            "320",
            "--content-width",
            "50",
        ]);
        let peek = params_of(&driven).peek(&driven);
        assert_eq!(peek.state, PeekState::Peeking);
        assert!(peek.is_right);
        assert_eq!(peek.peek_width, px(320.0));
        assert_eq!(peek.content_width, px(50.0));
    }

    #[test]
    fn state_parses_its_closed_vocabulary() {
        assert_eq!(parse_state("docked"), Ok(PeekState::Docked));
        assert_eq!(parse_state("closed"), Ok(PeekState::Closed));
        assert_eq!(parse_state("peeking"), Ok(PeekState::Peeking));
        assert!(matches!(parse_state("hidden"), Err(ParseError::Rejected(_))));
    }

    #[test]
    fn a_zero_height_is_rejected_by_name() {
        let line = ["--surface", "sidebar-peek", "--height", "0"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

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
        assert!(Params::default().no_state_axis());
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--state", "--right", "--peek-width", "--content-width"] {
            let line = ["--surface", "skeleton", option];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a skeleton option",
            );
        }
    }

    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--state", "--right", "--peek-width", "--content-width", "--height"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_its_root_and_is_full_bleed() {
        assert_eq!(SURFACE.name, "sidebar-peek");
        assert_eq!(SURFACE.root, "sidebar-peek");
        assert!(SURFACE.full_bleed);
    }
}
