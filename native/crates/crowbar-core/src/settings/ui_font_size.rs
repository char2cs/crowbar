//! Clamp/snap/scale one numeric field: the whole-UI chrome font size.
//!
//! Ported from `web/src/features/settings/lib/ui-font-size.ts`, all four
//! exports.

use super::raw_value::RawNumber;
use super::typography::DEFAULT_UI_FONT_SIZE;

pub const UI_FONT_SIZE_MIN: f64 = 10.0;
pub const UI_FONT_SIZE_MAX: f64 = 24.0;
pub const UI_FONT_SIZE_STEP: f64 = 0.5;
pub const UI_FONT_SIZE_DEFAULT: f64 = DEFAULT_UI_FONT_SIZE;

const UI_FONT_SCALE_PRECISION: i32 = 4;

/// Mirrors `normalizeUiFontSize(value: unknown)`. See
/// `normalize_markdown_font_size`'s doc for the shared `unknown`-narrowing
/// story; the differences here are the step (`0.5`, not whole pixels) and
/// the final `.toFixed(2)`-equivalent rounding to 2 decimal places, which
/// the TS source applies even though a `0.5` step can never actually
/// produce more than one decimal digit — kept for exactness with the
/// source rather than reasoned away.
#[must_use]
pub fn normalize_ui_font_size(value: Option<RawNumber<'_>>) -> f64 {
    let parsed = value.map_or(f64::NAN, RawNumber::as_f64);
    if !parsed.is_finite() {
        return UI_FONT_SIZE_DEFAULT;
    }

    let snapped = (parsed / UI_FONT_SIZE_STEP).round() * UI_FONT_SIZE_STEP;
    let clamped = snapped.clamp(UI_FONT_SIZE_MIN, UI_FONT_SIZE_MAX);

    round_to(clamped, 2)
}

/// Mirrors `shiftUiFontSize` — one step up (`direction = 1`) or down
/// (`direction = -1`), clamped, from a size that is itself normalized
/// first (so a caller can pass an out-of-range `currentSize` safely).
#[must_use]
pub fn shift_ui_font_size(current_size: f64, direction: i8) -> f64 {
    let next = normalize_ui_font_size(Some(RawNumber::Number(current_size)))
        + f64::from(direction) * UI_FONT_SIZE_STEP;
    normalize_ui_font_size(Some(RawNumber::Number(next)))
}

/// Mirrors `formatUiFontSize` — a normalized size, formatted to 2 decimal
/// places for display.
#[must_use]
pub fn format_ui_font_size(value: f64) -> String {
    format!(
        "{:.2}",
        normalize_ui_font_size(Some(RawNumber::Number(value)))
    )
}

/// Mirrors `getUiFontScale` — the ratio of a normalized size to the
/// default, rounded to [`UI_FONT_SCALE_PRECISION`] decimal places (e.g. for
/// scaling other chrome dimensions proportionally to the chosen UI font
/// size).
#[must_use]
pub fn get_ui_font_scale(value: f64) -> f64 {
    let normalized = normalize_ui_font_size(Some(RawNumber::Number(value)));
    round_to(normalized / UI_FONT_SIZE_DEFAULT, UI_FONT_SCALE_PRECISION)
}

/// `Number(value.toFixed(precision))` — round-half-away-from-zero to
/// `precision` decimal places, expressed without a string round-trip.
fn round_to(value: f64, precision: i32) -> f64 {
    let factor = 10f64.powi(precision);
    (value * factor).round() / factor
}

#[cfg(test)]
mod tests {
    use super::{
        UI_FONT_SIZE_DEFAULT, UI_FONT_SIZE_MAX, UI_FONT_SIZE_MIN, format_ui_font_size,
        get_ui_font_scale, normalize_ui_font_size, shift_ui_font_size,
    };
    use crate::settings::raw_value::RawNumber;

    fn assert_close(got: f64, want: f64) {
        assert!((got - want).abs() < 1e-9, "got {got}, want {want}");
    }

    // Always `Some` by design — see the identical helper in
    // `markdown_font_size.rs`'s test module for why.
    #[allow(clippy::unnecessary_wraps)]
    fn num(n: f64) -> Option<RawNumber<'static>> {
        Some(RawNumber::Number(n))
    }

    // --- ported from web/src/__tests__/features/settings/ui-font-size.test.ts ---

    #[test]
    fn uses_default_size_for_invalid_values() {
        assert_close(normalize_ui_font_size(None), UI_FONT_SIZE_DEFAULT);
        assert_close(
            normalize_ui_font_size(Some(RawNumber::Text("invalid"))),
            UI_FONT_SIZE_DEFAULT,
        );
    }

    #[test]
    fn snaps_values_to_half_pixel_increments_and_clamps_to_range() {
        assert_close(normalize_ui_font_size(num(14.26)), 14.5);
        assert_close(normalize_ui_font_size(num(9.2)), UI_FONT_SIZE_MIN);
        assert_close(normalize_ui_font_size(num(99.0)), UI_FONT_SIZE_MAX);
    }

    #[test]
    fn increments_and_decrements_by_one_step() {
        assert_close(shift_ui_font_size(14.0, 1), 14.5);
        assert_close(shift_ui_font_size(14.0, -1), 13.5);
    }

    #[test]
    fn does_not_move_outside_min_and_max_bounds() {
        assert_close(shift_ui_font_size(UI_FONT_SIZE_MIN, -1), UI_FONT_SIZE_MIN);
        assert_close(shift_ui_font_size(UI_FONT_SIZE_MAX, 1), UI_FONT_SIZE_MAX);
    }

    #[test]
    fn formats_values_with_two_decimals_and_exposes_stable_scale() {
        assert_eq!(format_ui_font_size(14.0), "14.00");
        assert_close(get_ui_font_scale(UI_FONT_SIZE_DEFAULT), 1.0);
        assert_close(
            get_ui_font_scale(UI_FONT_SIZE_DEFAULT + 2.5),
            ((UI_FONT_SIZE_DEFAULT + 2.5) / UI_FONT_SIZE_DEFAULT * 10000.0).round() / 10000.0,
        );
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn a_value_exactly_at_each_bound_survives_unclamped() {
        assert_close(
            normalize_ui_font_size(num(UI_FONT_SIZE_MIN)),
            UI_FONT_SIZE_MIN,
        );
        assert_close(
            normalize_ui_font_size(num(UI_FONT_SIZE_MAX)),
            UI_FONT_SIZE_MAX,
        );
    }

    #[test]
    fn parses_a_numeric_string_matching_markdown_font_sizes_own_behaviour() {
        // 15.25 / 0.5 = 30.5, which rounds away from zero to 31, snapping to
        // 15.5 — not the nearer-looking 15.0.
        assert_close(normalize_ui_font_size(Some(RawNumber::Text("15.25"))), 15.5);
    }
}
