//! `--surface sidebar-toast-overlay-fallback` — the `Toast.Portal`'d,
//! fixed-corner viewport `sidebarOpen === false` renders instead.
//!
//! See `crowbar_ui::surfaces::sidebar::sidebar_toast_overlay` for the port (this
//! surface calls its `render_fallback`) and
//! `native/mapping/sidebar-toast-overlay.md` for the measurement. The
//! sibling surface `sidebar-toast-overlay` is the inline half of the same
//! React component — see that module's own docs for why the registry's
//! unique-root constraint (`surface.rs`'s own doc comment) makes this two
//! entries rather than one with a flag.
//!
//! # `full_bleed`, and why — `fps_overlay`'s own reason, not `dialog`'s
//!
//! This is not "the viewport fills the window": `fixed bottom-4 left-4`/
//! `right-4` is a small, `w-72`-authored corner box, nothing like a
//! viewport-filling backdrop. It qualifies on `Surface::full_bleed`'s own
//! second, narrower door: taffy resolves `position: absolute` against the
//! immediate parent unconditionally, and this crate's row-layout harness
//! marks no ancestor `.relative()` — so, exactly as `fps_overlay.rs`'s own
//! module docs found first, `.absolute()` here resolves against the window
//! itself, the same containing-block fallback CSS `fixed` takes when nothing
//! establishes one. For `.bottom()`/`.left()`/`.right()` to land against the
//! window's **true** edges, the immediate parent has to be given the full,
//! uninset window width — which is exactly what `full_bleed` provides by
//! zeroing `crate::row_surface::INSET_X`. `row_layout::sidebar_toast_
//! overlay_fallback::the_viewport_docks_to_the_true_window_corner` is what
//! proves this rather than assumes it, `fps_overlay`'s own layout test one
//! door over.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width`/`--viewport-width` | **vacuous on the viewport's own box** — `w-72` is authored, and no `sm:` rule exists anywhere on this component. `--viewport-width` still matters for *where* the true window edge is, which is the whole point of `full_bleed` |
//! | `--content` | **vacuous** — same reason as the sibling surface |
//! | `--theme` | **vacuous on the viewport itself** |
//!
//! # The state axis
//!
//! Identical posture to the sibling surface: `empty` is real (an empty queue
//! renders no children), `--toasts` and `--side` are this surface's own
//! options, and every §8.3 interaction flag is unmodelled for the identical
//! reasons.
//!
//! **Uncapped, unlike the sibling surface** —
//! `crowbar_ui::surfaces::sidebar::sidebar_toast_overlay`'s own module docs §2:
//! only the inline viewport windows through `select_visible`. `--toasts
//! outage` on this surface therefore renders **all five** fixture toasts,
//! not three.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::sidebar::sidebar_toast_overlay::{self, Side, SidebarToastOverlay};
use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-toast-overlay-fallback",
    root: sidebar_toast_overlay::ID_VIEWPORT_FALLBACK,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The viewport is content-sized (no `--height` axis; see the module
    // docs), so this is a floor for the harness's caption, not a real
    // number this surface drives to.
    min_window_height: 120,
    // See the module docs' headline section.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// Which of the two fixture queues drives this cell.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
enum Preset {
    #[default]
    Single,
    Outage,
}

/// This surface's own options.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Params {
    /// `--side`: `sidebarSide`, which corner the viewport docks to.
    pub side: Side,
    preset: Preset,
}

impl Params {
    /// The overlay this cell describes — **uncapped**, module docs.
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

fn parse_side(raw: &str) -> Result<Side, ParseError> {
    match raw {
        "left" => Ok(Side::Left),
        "right" => Ok(Side::Right),
        _ => Err(ParseError::Rejected(format!("--side takes left or right, not {raw}"))),
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--side" => {
                let raw = value(args, option)?;
                self.side = parse_side(&raw)?;
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

    /// **None.** The viewport is content-sized — see the module docs.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · side {}",
            match self.side {
                Side::Left => "left",
                Side::Right => "right",
            },
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: no toasts");
        } else {
            let _ = write!(
                out,
                " · toasts {} (uncapped)",
                match self.preset {
                    Preset::Single => "single",
                    Preset::Outage => "outage",
                },
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        stage(cell)
            .child(self.overlay(cell).render_fallback(self.side, theme, anchors))
            .into_any_element()
    }
}

fn options() -> Vec<(String, String)> {
    vec![
        (
            "--side <left|right>".to_owned(),
            "sidebarSide — which corner the viewport docks to [left]".to_owned(),
        ),
        (
            "--toasts <single|outage>".to_owned(),
            "which fixture queue drives this cell, uncapped (module docs) [single]".to_owned(),
        ),
    ]
}

/// A stage exactly [`crate::row_surface::INSET_Y`] plus [`Cell::window_extent`]
/// tall — the room below the harness's own top inset — so the viewport's
/// `.absolute().bottom(…).left(…)`/`.right(…)` resolves against the
/// window's true edges rather than against `render_row`'s own zero-height
/// wrapper. `fps_overlay.rs`'s own `stage` function, verbatim: taffy
/// resolves `position: absolute` against the immediate parent
/// unconditionally, and an out-of-flow child contributes nothing to that
/// parent's own auto-height, so an unstaged wrapper collapses toward zero
/// and the viewport lands off the wrong edge. See the module docs.
fn stage(cell: &Cell) -> Div {
    div()
        .w(cell.width_px())
        .h(px(crate::row_surface::INSET_Y + f32::from(cell.window_extent())))
}

#[cfg(test)]
mod tests {
    use super::{Params, Preset, parse_side};
    use crate::row_surface::{Cell, StateFlag};
    use crowbar_ui::surfaces::sidebar::sidebar_toast_overlay::{SIDEBAR_TOAST_LIMIT, Side};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-toast-overlay-fallback"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// `--side` accepts exactly its two words.
    #[test]
    fn side_parses_its_two_words_and_rejects_everything_else() {
        assert_eq!(parse_side("left"), Ok(Side::Left));
        assert_eq!(parse_side("right"), Ok(Side::Right));
        assert!(parse_side("top").is_err());
    }

    /// **The whole point of this surface's separate existence**: `--toasts
    /// outage` here carries every fixture toast, not the sibling surface's
    /// windowed three.
    #[test]
    fn outage_is_uncapped_here() {
        let params = Params { preset: Preset::Outage, ..Params::default() };
        let overlay = params.overlay(&Cell::default());
        assert!(
            overlay.toasts.len() > SIDEBAR_TOAST_LIMIT,
            "the fallback viewport must not window: {} toasts against a limit of {}",
            overlay.toasts.len(),
            SIDEBAR_TOAST_LIMIT,
        );
        assert_eq!(overlay.toasts.len(), 5);
    }

    /// `empty` still wins over the preset here, identically to the sibling
    /// surface.
    #[test]
    fn empty_wins_over_the_preset() {
        let params = Params { preset: Preset::Outage, ..Params::default() };
        let empty_cell = cell(&["--flags", "empty"]);
        assert!(empty_cell.has(StateFlag::Empty));
        let overlay = params.overlay(&empty_cell);
        assert!(overlay.toasts.is_empty());
    }
}
