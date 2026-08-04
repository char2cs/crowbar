//! `--surface sidebar-toast-overlay` — the inline (`sidebarOpen`) viewport,
//! docked to the sidebar column's own bottom edge.
//!
//! See `crowbar_ui::surfaces::sidebar::sidebar_toast_overlay` for the port and the
//! four findings this item's brief asked to be verified, and
//! `native/mapping/sidebar-toast-overlay.md` for the measurement. The
//! `Toast.Portal`/fixed-corner half of this one React component is the
//! sibling surface `sidebar-toast-overlay-fallback` — see that component's
//! own module docs §1 for why the registry's unique-root constraint makes
//! this two entries rather than one with a flag.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real** — the viewport's `w-full` stretches with it |
//! | `--height` | **real, and drives the window** — the sidebar column this viewport is docked inside; `sidebar-carousel`'s own `--height` axis, applied to a second surface that needs the same fiction of a surrounding column |
//! | `--content` | **vacuous** — no anchor here paints text at all (the queue's own content is unanchored, module docs) |
//! | `--theme` | **vacuous on the viewport itself** — it paints no fill of its own; every colour lives on the unanchored items inside it |
//! | `--viewport-width` | **vacuous** — no `sm:` rule anywhere on this component |
//!
//! # The state axis
//!
//! `empty` is real: an empty queue renders the viewport with no children at
//! all — `toasts.length === 0`, the one real content state this surface's
//! anchor set does not depend on (the anchor set is always just the one
//! viewport, empty or full). `--toasts` (`single`/`outage`) is this
//! surface's own option rather than a §8.3 word — the queue's own shape has
//! no term in that vocabulary, the same reasoning `--held` takes on
//! `placeholder-row-actions`.
//!
//! `hover`/`focus`/`selected`/`loading`/`error` are all unmodelled: the
//! viewport itself carries no interactive rule of its own, and every state a
//! real toast item has (its own `hover:opacity-100` close button, in
//! particular) belongs to unanchored content this surface does not measure.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::sidebar::sidebar_toast_overlay::{self, SidebarToastOverlay};
use gpui::{AnyElement, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-toast-overlay",
    root: sidebar_toast_overlay::ID_VIEWPORT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // `--height`'s own default (600, `sidebar_carousel`'s own number for the
    // identical fiction) plus `CAPTION_HEIGHT`'s 29 — a floor, since the
    // window follows `--height` past it (`driven_height` below).
    min_window_height: 630,
    // Inset inside the sidebar column, not flush chrome.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `sidebar_carousel::DEFAULT_HEIGHT`'s own number, restated rather than
/// imported — two different surfaces' own fiction of "the column I am docked
/// inside," not one shared quantity.
const DEFAULT_HEIGHT: u16 = 600;

/// Which of the two fixture queues drives this cell.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
enum Preset {
    #[default]
    Single,
    Outage,
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--height`: the sidebar column's own height, and what the window
    /// follows.
    pub height: u16,
    preset: Preset,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            height: DEFAULT_HEIGHT,
            preset: Preset::default(),
        }
    }
}

impl Params {
    /// The overlay this cell describes.
    #[must_use]
    pub fn overlay(&self, cell: &Cell) -> SidebarToastOverlay {
        if cell.has(StateFlag::Empty) {
            return SidebarToastOverlay { toasts: Vec::new() };
        }
        match self.preset {
            Preset::Single => SidebarToastOverlay::fixture_single(),
            Preset::Outage => SidebarToastOverlay::fixture_outage(),
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
            "--height" => {
                let height = pixels(&value(args, option)?, option)?;
                if height == 0 {
                    return Err(ParseError::Rejected(
                        "--height must be greater than zero: a column with no height leaves \
                         nothing to dock the viewport against"
                            .to_owned(),
                    ));
                }
                self.height = height;
            }
            "--toasts" => {
                let raw = value(args, option)?;
                self.preset = match raw.as_str() {
                    "single" => Preset::Single,
                    "outage" => Preset::Outage,
                    _ => {
                        return Err(ParseError::Rejected(format!(
                            "--toasts takes single or outage, not {raw}"
                        )));
                    }
                };
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// `--height` — the column the viewport is docked to, so it **is** the
    /// surface's height, and the window follows it (`sidebar_carousel`'s own
    /// reasoning).
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        Some(self.height)
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · column height {}", self.height);
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no toasts");
        } else {
            let _ = write!(
                out,
                " · toasts {}",
                match self.preset {
                    Preset::Single => "single",
                    Preset::Outage => "outage",
                },
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.overlay(cell)
            .render_inline(px(f32::from(self.height)), theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    vec![
        (
            "--height <px>".to_owned(),
            format!(
                "the sidebar column's own height, and what the window follows [{DEFAULT_HEIGHT}]"
            ),
        ),
        (
            "--toasts <single|outage>".to_owned(),
            "which fixture queue drives this cell — outage is the pinned-toast-survives shape \
             [single]"
                .to_owned(),
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_HEIGHT, Params, Preset};
    use crate::row_surface::Cell;
    use crate::surface::SurfaceParams;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-toast-overlay"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// `--height` rejects zero, by name.
    #[test]
    fn height_rejects_zero() {
        let mut params = Params::default();
        let err = params.accept("--height", &mut vec!["0".to_owned()].into_iter());
        assert!(err.is_err());
    }

    /// `--toasts` accepts exactly its two words.
    #[test]
    fn toasts_parses_its_two_words_and_rejects_everything_else() {
        let mut params = Params::default();
        assert!(params.accept("--toasts", &mut vec!["single".to_owned()].into_iter()).is_ok());
        assert_eq!(params.preset, Preset::Single);
        assert!(params.accept("--toasts", &mut vec!["outage".to_owned()].into_iter()).is_ok());
        assert_eq!(params.preset, Preset::Outage);
        assert!(
            params
                .accept("--toasts", &mut vec!["everything".to_owned()].into_iter())
                .is_err()
        );
    }

    /// `empty` returns an empty queue regardless of `--toasts`; otherwise
    /// the preset reaches the overlay unchanged.
    #[test]
    fn empty_wins_over_the_preset() {
        let mut params = Params { preset: Preset::Outage, ..Params::default() };
        let overlay = params.overlay(&cell(&["--flags", "empty"]));
        assert!(overlay.toasts.is_empty());

        params.preset = Preset::Single;
        let overlay = params.overlay(&cell(&[]));
        assert_eq!(overlay.toasts.len(), 1);
    }

    /// `--height`'s default is the sidebar column fiction's own number.
    #[test]
    fn the_default_height_is_600() {
        assert_eq!(DEFAULT_HEIGHT, 600);
        assert_eq!(Params::default().height, DEFAULT_HEIGHT);
    }
}
