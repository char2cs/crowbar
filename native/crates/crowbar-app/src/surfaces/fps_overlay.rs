//! `--surface fps-overlay` — the performance badge, fixed to the window's own
//! bottom-right corner.
//!
//! **No reference exists for this surface at all** — `fps-overlay.tsx` carries
//! no `data-oracle-id` anywhere in its own file, so there is no anchor id the
//! React extractor could ever collect here in any build. See
//! `crowbar_ui::components::fps_overlay`'s module docs and
//! `native/mapping/fps-overlay.md`.
//!
//! # The state axis, and why this is a `no_state_axis` surface
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `fps-overlay.tsx` has no `hover`/`focus`/`selected`/`empty`/`loading`/`error` original: `pointer-events-none`, `aria-hidden`, and its own content is three independently-guarded numbers rather than a §8.3-shaped state. `--fps`/`--max-dt`/`--drops` are this surface's own axis instead, the same way `tabs`' `--tabs`/`--active` are. |
//!
//! Checked exhaustively, the same way `workspace-branch-icon`'s own module
//! docs do: `export function FpsOverlay()` takes **no props at all**, not
//! even a `status`/`working`-shaped real prop left unforwarded — every
//! `className` on every element `fps-overlay.tsx` renders is a fixed
//! string, there is no prop spread anywhere, and `Empty`'s §8.3 meaning (a
//! *row's* trailing edge with no badge or count, modelled on
//! `git-status-row` alone) does not apply to a fixed-position badge that is
//! not a row. So there is no edge value left to reach through any
//! hypothetical caller — see `crowbar_ui::components::fps_overlay`'s own
//! module docs for the component-side half of the same check.
//! [`SurfaceParams::no_state_axis`] below is that declaration.
//!
//! # `full_bleed`, and why: not because the badge fills the window — because
//! its positioning **context** has to
//!
//! **This is not `dialog`'s/`command`'s reason.** Their own reference is a
//! `fixed inset-0` backdrop that genuinely *is* viewport-width; `fps-overlay.tsx`'s
//! own `<div>` is `fixed bottom-8 right-3` — a small, content-sized corner
//! badge, nothing like a backdrop. Read `Surface::full_bleed`'s own doc
//! comment before assuming this surface therefore does not qualify: it
//! names this as the second, narrower shape the flag also covers. Taffy
//! resolves `position: absolute` against the **immediate parent
//! unconditionally**, not against the nearest CSS-positioned ancestor and
//! not against the window directly the way CSS `position: fixed` falls back
//! to when nothing establishes a containing block (`row_layout::fps_overlay`
//! proves this the hard way — see below). So for the badge's `.bottom()`/
//! `.right()` to land against the window's *true* edges, its own immediate
//! parent — this file's own `stage` function — has to be given the full, uninset
//! window width to sit in, which is exactly what `full_bleed` provides by
//! zeroing [`crate::row_surface::INSET_X`]. The `row_layout` test drives
//! `--width`/`--viewport-width` equal for the same reason `dialog`'s and
//! `command`'s do — a full-bleed surface's own declared width **is** the
//! viewport's — even though what that width sizes here is the stage, not
//! anything `fps-overlay.tsx` itself paints. Dropping the flag was checked,
//! not assumed: it moves `stage`'s own origin [`crate::row_surface::INSET_X`]
//! px right of the window's true left edge without moving its declared
//! width to compensate, which moves the badge's landing point by the same
//! amount — a real geometry change, confirmed by the arithmetic below
//! before it was confirmed by a run.
//!
//! # The stage this surface builds, and why: taffy's containing block is the
//! **immediate parent**, always — not "the nearest `.relative()` ancestor"
//!
//! Measured, not assumed — `row_layout::fps_overlay` is what proved it.
//! `render_row`'s own wrapper (`div().w(cell.width_px())`, no explicit
//! height) never carries `.relative()`, and a first attempt at this surface
//! that returned the badge alone anyway resolved `.bottom()`/`.right()`
//! against **that wrapper**, not the window: with no other content, an
//! absolutely-positioned child contributes nothing to its parent's own
//! auto-height, so the wrapper collapsed toward zero and the badge's `bottom:
//! 32px` landed 32px *above* a near-zero-height box — off the top of the
//! window entirely, reported `visible: false`. taffy's containing block is
//! the immediate parent unconditionally, not "the nearest ancestor CSS would
//! call positioned" the way `native/…/gpui/references/layout-style.md`'s
//! `.relative()` example reads.
//!
//! So this surface hands the component a **stage sized to the room the
//! harness's own top inset leaves below it** — [`crate::row_surface::INSET_Y`]
//! plus [`Cell::window_extent`], which lands the stage's own bottom edge
//! exactly on the window's true bottom (the harness pads only the *top*, so
//! nothing is lost tracking the other three edges). In the real app this
//! surface mounts under `IDEShell`'s own window-sized root, where the
//! question this section answers never comes up — it is purely an artefact
//! of the `row_layout` harness's own wrapping, and it belongs here rather than
//! in `crowbar_ui::components::fps_overlay`, which stays agnostic of it.

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::fps_overlay::{FpsOverlay, FrameStats};
use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "fps-overlay",
    root: crowbar_ui::components::fps_overlay::ID_FPS_OVERLAY,
    unmodelled: &[
        // All four non-mandatory flags, including `Empty` — this component
        // has no seam of any kind, checked exhaustively (module docs), so
        // there is no edge value left to reach through. `Params::
        // no_state_axis` below is the declaration that makes this
        // deliberate rather than an oversight.
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // Room for the badge (one padded line) plus its 32px clearance off the
    // window's bottom edge, plus the harness's own top inset and caption.
    min_window_height: 120,
    // `fixed bottom-8 right-3` — see the module docs.
    full_bleed: true,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--fps`.
    pub fps: u32,
    /// `--max-dt`.
    pub max_dt_ms: u32,
    /// `--drops`.
    pub drops: u32,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = FrameStats::fixture();
        Self {
            fps: fixture.fps,
            max_dt_ms: fixture.max_dt_ms,
            drops: fixture.drops,
        }
    }
}

impl Params {
    /// The badge this cell describes.
    #[must_use]
    pub fn overlay(&self) -> FpsOverlay {
        FpsOverlay {
            stats: FrameStats {
                fps: self.fps,
                max_dt_ms: self.max_dt_ms,
                drops: self.drops,
            },
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
            "--fps" => self.fps = parse_count(&crate::surface::value(args, option)?, option)?,
            "--max-dt" => {
                self.max_dt_ms = parse_count(&crate::surface::value(args, option)?, option)?;
            }
            "--drops" => self.drops = parse_count(&crate::surface::value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Nothing on this command line sets a height: the badge's own
    /// is padding around whatever its runs measure, and its position is fixed
    /// off the window's own edges regardless of `--width`.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        use std::fmt::Write as _;

        let overlay = self.overlay();
        let _ = write!(
            out,
            " · {} fps ({}) · max {} · {} drops",
            overlay.stats.fps_text(),
            overlay.stats.tier().name(),
            overlay.stats.max_dt_text(),
            overlay.stats.drops,
        );
        out.push_str(
            " · no reference: fps-overlay.tsx carries no data-oracle-id anywhere in its \
             own file",
        );
        let _ = cell; // every flag on this surface is unmodelled; nothing else to say per-cell
    }

    /// **`true`, unconditionally.** See the module docs and this surface's
    /// own `unmodelled` list: every §8.3 flag is checked and none applies —
    /// `fps-overlay.tsx` takes no props at all, so there is no `--flags`
    /// branch, real or reachable-by-construction, left for a cell to drive
    /// on this surface.
    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        stage(cell)
            .child(self.overlay().render(theme, anchors))
            .into_any_element()
    }
}

/// A stage exactly [`crate::row_surface::INSET_Y`] plus [`Cell::window_extent`]
/// tall — the room below the harness's own top inset — so `FpsOverlay`'s
/// `.absolute().bottom(…).right(…)` resolves against the window's true edges
/// rather than against `render_row`'s own zero-height wrapper. See the module
/// docs.
fn stage(cell: &Cell) -> Div {
    div()
        .w(cell.width_px())
        .h(px(crate::row_surface::INSET_Y + f32::from(cell.window_extent())))
}

/// A non-negative frame-timing count: an `fps`, a `max-dt` in ms, or a
/// `drops` tally. All three are the same shape of number, so one parser
/// serves all three options.
fn parse_count(raw: &str, option: &str) -> Result<u32, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("{option} takes a non-negative count, not {raw}")))
}

fn options() -> Vec<(String, String)> {
    let fixture = Params::default();
    [
        (
            "--fps <n>".to_owned(),
            format!("0 renders the dash fallback [{}]", fixture.fps),
        ),
        (
            "--max-dt <n>".to_owned(),
            format!("in ms; 0 renders the dash fallback [{}]", fixture.max_dt_ms),
        ),
        (
            "--drops <n>".to_owned(),
            format!(
                "0 stays muted; any other value paints destructive+medium [{}]",
                fixture.drops,
            ),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::fps_overlay::{FpsTier, ID_FPS_OVERLAY};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "fps-overlay"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("an fps-overlay cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_components_own_first_paint() {
        let bag = Params::default();
        assert_eq!(bag.fps, 0);
        assert_eq!(bag.max_dt_ms, 0);
        assert_eq!(bag.drops, 0);
        assert_eq!(params_of(&cell(&[])).overlay().stats.tier(), FpsTier::Idle);
    }

    /// The three options reach the component, independently.
    #[test]
    fn the_three_options_reach_the_stats_independently() {
        let driven = params_of(&cell(&["--fps", "120", "--max-dt", "7", "--drops", "2"])).overlay();
        assert_eq!(driven.stats.fps, 120);
        assert_eq!(driven.stats.max_dt_ms, 7);
        assert_eq!(driven.stats.drops, 2);
        assert_eq!(driven.stats.tier(), FpsTier::Good);

        let bad = params_of(&cell(&["--fps", "40"])).overlay();
        assert_eq!(bad.stats.tier(), FpsTier::Bad);
        // Unset options keep the fixture's own zero.
        assert_eq!(bad.stats.max_dt_ms, 0);
        assert_eq!(bad.stats.drops, 0);
    }

    #[test]
    fn a_non_numeric_count_is_rejected_by_name() {
        for line in [
            vec!["--fps", "fast"],
            vec!["--fps", "-1"],
            vec!["--max-dt", "slow"],
            vec!["--drops", "many"],
        ] {
            let mut full = vec!["--surface", "fps-overlay"];
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

    /// Every flag on this surface is unmodelled, and the surface declares
    /// that on purpose — the two facts
    /// `no_surface_declares_its_entire_state_axis_unmodelled` (`surface.rs`)
    /// requires to agree with each other. There is no state axis at all;
    /// `--fps`/`--max-dt`/`--drops` are this surface's own instead.
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
        for option in ["--fps", "--max-dt", "--drops"] {
            let line = ["--surface", "skeleton", option, "10"];
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
        for option in ["--fps", "--max-dt", "--drops"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "fps-overlay");
        assert_eq!(SURFACE.root, ID_FPS_OVERLAY);
        assert_eq!(SURFACE.root, "fps-overlay");
        assert!(SURFACE.full_bleed);
    }
}
