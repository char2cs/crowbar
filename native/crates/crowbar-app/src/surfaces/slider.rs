//! `--surface slider` — the second genuinely style-only widget (`switch` the
//! first), and the first surface whose `--width` is a **real** axis rather
//! than a vacuous one.
//!
//! One `<Slider>` in one cell of the §8.3 matrix. The default cell is the live
//! Settings → Developer → "Fault Injection" rows —
//! `web/src/features/settings/components/tabs/developer-settings.tsx`,
//! `<Slider value={…} min={0} max={100} step={5} className="min-w-0 flex-1" />`
//! — measured at `668px` (the row's own width) at `innerWidth` 1714.
//!
//! See `crowbar_ui::components::slider`'s module docs for the wrap-or-build
//! seam test and the full derivation, and `native/mapping/slider.md` for the
//! §6.2 row and the pseudo-inset finding.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real, and the whole reason this surface is worth having**: `slider.tsx`'s `Control` is `w-full`, so this surface's `--width` is the track's own width, not a column an authored component sits inside (`number_input`'s own vacuous case, inverted) |
//! | `--viewport-width` | real: `size-5 sm:size-4` moves the thumb's extent and, through it, the inset both the indicator and the thumb are computed from |
//! | `--theme` | real, but **not uniformly** — `bg-input` (track) and the thumb's border move; `bg-primary` (indicator) and the thumb's `bg-white` fill do not. See `crowbar_ui::components::slider`'s colour docs |
//! | `--content` | **vacuous.** No anchor on this surface paints text |
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `focus` | real in the class list (`has-focus-visible:ring-*`) and **invisible**: a ring is a box-shadow, `ANCHORS.md` §6. Also unreachable — `document.hasFocus()` is false and immovable on this machine |
//! | `hover`, `selected`, `empty`, `loading`, `error` | unmodelled — `slider.tsx` contains none of `hover`, `selected`, `aria-invalid`; a slider has no "empty" content to be empty of, and `loading` is unmodelled everywhere |
//!
//! `--value`/`--min`/`--max` are this surface's own knobs, standing in for
//! `§8.3`'s `selected` the way a continuous control has to: there is no single
//! boolean "the slider is on," only where its thumb sits.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::slider::Slider;
use crowbar_ui::components::{AnchorSink, slider};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "slider",
    root: slider::ID_ROOT,
    unmodelled: &[
        StateFlag::Hover,
        StateFlag::Selected,
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
    ],
    // The tallest thumb is the **base** breakpoint's 20px, centred on a 4px
    // row — 8px above it, 8px below — plus `CAPTION_HEIGHT`'s 29. 64 holds
    // that with room, and is a floor rather than a ceiling: nothing on this
    // surface drives a height, since the row's own extent is fixed by the
    // thumb's centring rather than by anything `--content` could grow.
    min_window_height: 64,
    // A slider sits inside a settings row — never the window itself.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options: the value and its range, plus `--disabled`.
///
/// Not a `Driven` struct, for `switch::Params`'s reason: these are not one
/// kind of thing. `--value`/`--min`/`--max` stand in for §8.3's `selected` —
/// see the module docs — and `--disabled` is the prop.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Params {
    /// `--min`: the value's lower bound. `0.0` at the one live call site.
    pub min: f32,
    /// `--max`: the value's upper bound. `100.0` at the one live call site.
    pub max: f32,
    /// `--value`: the current value. `0.0` is the resting cell — the live
    /// reference (`/tmp/p3-ref-slider.json`) was captured from it.
    pub value: f32,
    /// `--disabled`: the `disabled` prop. No live call site passes it, and
    /// it is invisible either way (`opacity-64` never reaches v1.7's zero).
    pub disabled: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Slider::fixture();
        Self {
            min: fixture.min,
            max: fixture.max,
            value: fixture.value,
            disabled: fixture.disabled,
        }
    }
}

impl Params {
    /// The slider this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than
    /// assembling one from scratch — so a bare `--surface slider` renders the
    /// slider the reference actually has, at the reference's own width.
    #[must_use]
    pub fn slider(&self, cell: &Cell) -> Slider {
        let mut slider = Slider::fixture();
        slider.width = cell.width_px();
        slider.breakpoint = cell.breakpoint();
        slider.min = self.min;
        slider.max = self.max;
        slider.value = self.value;
        slider.disabled = self.disabled;
        slider.focused = cell.has(StateFlag::Focus);
        slider
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--value" => self.value = parse_f32(&value(args, option)?, option)?,
            "--min" => self.min = parse_f32(&value(args, option)?, option)?,
            "--max" => self.max = parse_f32(&value(args, option)?, option)?,
            "--disabled" => self.disabled = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The row's own height is the thumb's extent centred on the
    /// track, and no option here sets one — [`Surface::min_window_height`] is
    /// the whole answer, as it is for `switch`.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let slider = self.slider(cell);

        let _ = write!(
            out,
            " · value {} of [{}, {}]",
            slider.value, slider.min, slider.max,
        );
        let _ = write!(
            out,
            " · thumb centre {}px of a {}px track (thumbAlignment=edge: inset by half the \
             thumb's own extent at both ends, not a plain percentage)",
            f32::from(slider.thumb_center()),
            f32::from(slider.width),
        );
        out.push_str(
            " · --width is real here: slider.tsx's Control is w-full, so this is the \
             track's own width, not a column an authored component sits inside",
        );
        out.push_str(
            " · a slider paints no text, so the reference emits no text/fg/font for any \
             anchor and --content cannot fail",
        );

        if self.disabled {
            out.push_str(
                " · disabled: opacity-64 has no field and does not reach v1.7's zero, so \
                 this cell cannot fail — and no live call site passes it",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: has-focus-visible:ring-* is a box-shadow, which ANCHORS.md §6 \
                 has no field for, so this cell cannot fail — and document.hasFocus() is \
                 false on this machine, so there is no reference either",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.slider(cell).render(theme, anchors)
    }
}

/// A finite `f32`, or a rejection naming what was given.
///
/// # Errors
///
/// [`ParseError::Rejected`] when `raw` is not a finite `f32`.
fn parse_f32(raw: &str, option: &str) -> Result<f32, ParseError> {
    let parsed: f32 = raw
        .parse()
        .map_err(|_| ParseError::Rejected(format!("{option} takes a number, not {raw}")))?;
    if !parsed.is_finite() {
        return Err(ParseError::Rejected(format!(
            "{option} takes a finite number, not {raw}"
        )));
    }
    Ok(parsed)
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--value".to_owned(),
            "the current value, between --min and --max [0]".to_owned(),
        ),
        ("--min".to_owned(), "the value's lower bound [0]".to_owned()),
        (
            "--max".to_owned(),
            "the value's upper bound [100]".to_owned(),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop: opacity-64, which the contract cannot see [off]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::Breakpoint;
    use crowbar_ui::components::slider::{ID_INDICATOR, ID_ROOT, ID_THUMB, ID_TRACK, Slider};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "slider"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a slider cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Slider {
        params_of(cell).slider(cell)
    }

    /// The defaults are the live fault-injection slider's own **params** —
    /// `value`/`min`/`max`/`disabled` — and the bag reproduces the reference
    /// exactly once its width matches the reference's too.
    ///
    /// Not `built(&cell(&[]))` against the bare fixture: unlike `switch`,
    /// this surface's `width` is genuinely driven by `--width` (`RowSurface`'s
    /// own default is `320`, not the reference's `668`), so a bare cell and
    /// the fixture agree on everything *except* the axis this surface exists
    /// to exercise.
    #[test]
    fn the_defaults_are_the_live_slider_at_rest() {
        let bag = Params::default();
        assert!((bag.value - 0.0).abs() < f32::EPSILON);
        assert!((bag.min - 0.0).abs() < f32::EPSILON);
        assert!((bag.max - 100.0).abs() < f32::EPSILON);
        assert!(!bag.disabled);

        let bare = built(&cell(&[]));
        assert_ne!(
            bare.width,
            Slider::fixture().width,
            "320 is the harness default, not 668"
        );
        assert_eq!(built(&cell(&["--width", "668"])), Slider::fixture());
    }

    /// **`--width` is real here** — the control that separates this surface
    /// from `switch`'s vacuous one and from `number_input`'s own vacuous case.
    ///
    /// Driven at a non-zero `--value`: at rest (`value: 0`) the thumb's centre
    /// is exactly half its own extent regardless of the track's width — the
    /// formula's own `value_fraction() == 0` term cancels the width out — so
    /// `--width` moving the centre is only observable once there is a value
    /// for it to scale.
    #[test]
    fn the_width_axis_reaches_the_track() {
        let narrow = built(&cell(&["--width", "200", "--value", "40"]));
        let wide = built(&cell(&["--width", "600", "--value", "40"]));

        assert_eq!(narrow.width, px(200.0));
        assert_eq!(wide.width, px(600.0));
        assert_ne!(narrow.thumb_center(), wide.thumb_center());
    }

    /// `--value` moves the thumb's centre, and the formula is `thumbAlignment
    /// = edge` — inset by half the thumb's own extent at both ends, not a
    /// plain percentage of the track.
    #[test]
    fn value_moves_the_thumb_and_the_edges_are_inset_by_half_the_thumb() {
        let at_min = built(&cell(&["--value", "0"]));
        let at_max = built(&cell(&["--value", "100"]));

        let inset = at_min.thumb_extent() * 0.5;
        assert_eq!(at_min.thumb_center(), inset);
        assert_eq!(at_max.thumb_center(), at_min.width - inset);
        assert_ne!(at_min.thumb_center(), at_max.thumb_center());
    }

    /// `--min`/`--max` move what a given `--value` means, not just its
    /// display — the range genuinely reaches the geometry.
    #[test]
    fn min_and_max_rescale_the_value() {
        let default_range = built(&cell(&["--value", "50"]));
        let narrow_range = built(&cell(&["--value", "50", "--min", "0", "--max", "50"]));

        assert!((default_range.value_fraction() - 0.5).abs() < f32::EPSILON);
        assert!((narrow_range.value_fraction() - 1.0).abs() < f32::EPSILON);
        assert_ne!(default_range.thumb_center(), narrow_range.thumb_center());
    }

    /// The caption says which axis is real and which cells cannot fail.
    #[test]
    fn the_caption_names_the_real_width_axis_and_the_vacuous_content_axis() {
        let described = cell(&[]).describe();
        assert!(described.contains("--width is real here"), "{described}");
        assert!(described.contains("--content cannot fail"), "{described}");
        assert!(described.contains("value 0 of [0, 100]"), "{described}");
    }

    /// The invisible states reach the component and each says in the caption
    /// that it cannot fail.
    #[test]
    fn the_invisible_states_reach_the_component_and_say_they_cannot_fail() {
        let disabled = built(&cell(&["--disabled"]));
        assert!(disabled.disabled);
        let caption = cell(&["--disabled"]).describe();
        assert!(caption.contains("disabled:"), "{caption}");
        assert!(caption.contains("cannot fail"), "{caption}");

        let focused = built(&cell(&["--flags", "focus"]));
        assert!(focused.focused);
        let caption = cell(&["--flags", "focus"]).describe();
        assert!(caption.contains("box-shadow"), "{caption}");
        assert!(caption.contains("no reference"), "{caption}");
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
    }

    /// The viewport axis reaches the component through the thumb's extent —
    /// the control that stops `--viewport-width` being decoration.
    ///
    /// `--width 200` (rather than a bare cell's `320`) so a sub-640 viewport
    /// is reachable at all — a wider surface needs a wider window than the
    /// `sm` breakpoint regardless of `--viewport-width`, `RowSurface`'s own
    /// two-inset floor. **`--value 40`, not `50`**: at exactly the midpoint
    /// the centre is `width / 2` at *any* thumb extent — `inset +
    /// 0.5 × (width − 2 × inset)` reduces to `width / 2` regardless of
    /// `inset` — so `50` would make the two breakpoints agree by algebra and
    /// prove nothing about the axis.
    #[test]
    fn the_viewport_axis_reaches_the_thumb() {
        let sm = built(&cell(&["--width", "200", "--value", "40"]));
        let base = built(&cell(&[
            "--width",
            "200",
            "--value",
            "40",
            "--viewport-width",
            "300",
        ]));

        assert_eq!(sm.breakpoint, Breakpoint::Sm);
        assert_eq!(base.breakpoint, Breakpoint::Base);
        assert_eq!(sm.thumb_extent(), px(16.0));
        assert_eq!(base.thumb_extent(), px(20.0));
        assert_ne!(sm.thumb_center(), base.thumb_center());
    }

    /// A malformed `--value`/`--min`/`--max` is rejected rather than silently
    /// taken as zero.
    #[test]
    fn a_malformed_number_is_rejected() {
        for option in ["--value", "--min", "--max"] {
            let full = ["--surface", "slider", option, "not-a-number"];
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option}",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("slider"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields, and the state axis this surface
    /// really has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "slider");
        assert_eq!(SURFACE.root, "slider");
        assert_eq!(SURFACE.root, ID_ROOT);
        assert!(!SURFACE.full_bleed);
        for id in [ID_TRACK, ID_INDICATOR, ID_THUMB] {
            assert_ne!(SURFACE.root, id);
        }

        assert!(!SURFACE.unmodelled(StateFlag::Focus));
        for flag in [
            StateFlag::Hover,
            StateFlag::Selected,
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }
}
