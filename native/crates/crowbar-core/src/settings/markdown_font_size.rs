//! Clamp/snap one numeric field: the markdown document surface's base type
//! size.
//!
//! Ported from `web/src/features/settings/lib/markdown-font-size.ts`.
//! Separate from the code `fontSize`: that one drives Monaco, including the
//! Source view of the very same markdown file, and a reading size and a
//! code size are not the same want.

use super::raw_value::RawNumber;
use super::typography::DEFAULT_MARKDOWN_FONT_SIZE;

pub const MARKDOWN_FONT_SIZE_MIN: f64 = 12.0;
pub const MARKDOWN_FONT_SIZE_MAX: f64 = 24.0;
pub const MARKDOWN_FONT_SIZE_STEP: f64 = 1.0;
pub const MARKDOWN_FONT_SIZE_DEFAULT: f64 = DEFAULT_MARKDOWN_FONT_SIZE;

/// Mirrors `normalizeMarkdownFontSize(value: unknown)`. Accepts either a
/// genuine number or a numeric string (a persisted value can arrive as
/// either — see [`RawNumber`]'s module doc); anything else, or a value that
/// doesn't parse to a finite number, falls back to
/// [`MARKDOWN_FONT_SIZE_DEFAULT`]. A finite value is snapped to whole pixels
/// (`MARKDOWN_FONT_SIZE_STEP` is `1`, unlike `ui_font_size`'s `0.5`) and
/// clamped to `[MARKDOWN_FONT_SIZE_MIN, MARKDOWN_FONT_SIZE_MAX]`.
#[must_use]
pub fn normalize_markdown_font_size(value: Option<RawNumber<'_>>) -> f64 {
    let parsed = value.map_or(f64::NAN, RawNumber::as_f64);
    if !parsed.is_finite() {
        return MARKDOWN_FONT_SIZE_DEFAULT;
    }

    let snapped = (parsed / MARKDOWN_FONT_SIZE_STEP).round() * MARKDOWN_FONT_SIZE_STEP;
    snapped.clamp(MARKDOWN_FONT_SIZE_MIN, MARKDOWN_FONT_SIZE_MAX)
}

#[cfg(test)]
mod tests {
    use super::{
        MARKDOWN_FONT_SIZE_DEFAULT, MARKDOWN_FONT_SIZE_MAX, MARKDOWN_FONT_SIZE_MIN,
        normalize_markdown_font_size,
    };
    use crate::settings::raw_value::RawNumber;

    fn assert_close(got: f64, want: f64) {
        assert!((got - want).abs() < 1e-9, "got {got}, want {want}");
    }

    // Always `Some` by design — a terser call site than spelling
    // `Some(RawNumber::Number(n))` at every one of this module's clamp-range
    // test cases; `None`/`RawNumber::Text` are exercised directly where the
    // test is specifically about that branch.
    #[allow(clippy::unnecessary_wraps)]
    fn num(n: f64) -> Option<RawNumber<'static>> {
        Some(RawNumber::Number(n))
    }

    // --- ported from web/src/__tests__/features/settings/lib/markdown-font-size.test.ts ---

    #[test]
    fn keeps_an_in_range_whole_pixel_value_untouched() {
        assert_close(normalize_markdown_font_size(num(18.0)), 18.0);
    }

    #[test]
    fn defaults_to_the_size_the_stylesheet_used_to_hardcode() {
        assert_close(MARKDOWN_FONT_SIZE_DEFAULT, 16.0);
    }

    #[test]
    fn clamps_below_the_minimum_and_above_the_maximum() {
        assert_close(
            normalize_markdown_font_size(num(2.0)),
            MARKDOWN_FONT_SIZE_MIN,
        );
        assert_close(
            normalize_markdown_font_size(num(999.0)),
            MARKDOWN_FONT_SIZE_MAX,
        );
    }

    #[test]
    fn snaps_fractional_input_to_whole_pixels() {
        assert_close(normalize_markdown_font_size(num(17.4)), 17.0);
        assert_close(normalize_markdown_font_size(num(17.6)), 18.0);
    }

    #[test]
    fn parses_a_numeric_string_as_a_persisted_or_typed_value_can_be() {
        assert_close(
            normalize_markdown_font_size(Some(RawNumber::Text("20"))),
            20.0,
        );
    }

    #[test]
    fn falls_back_to_the_default_for_anything_non_finite() {
        for bad in [
            num(f64::NAN),
            num(f64::INFINITY),
            None,
            Some(RawNumber::Text("huge")),
        ] {
            assert_close(
                normalize_markdown_font_size(bad),
                MARKDOWN_FONT_SIZE_DEFAULT,
            );
        }
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn a_value_exactly_at_each_bound_survives_unclamped() {
        // The clamp-boundary case the TS suite's below/above pair doesn't
        // cover: the bound itself must not be pushed further in.
        assert_close(
            normalize_markdown_font_size(num(MARKDOWN_FONT_SIZE_MIN)),
            MARKDOWN_FONT_SIZE_MIN,
        );
        assert_close(
            normalize_markdown_font_size(num(MARKDOWN_FONT_SIZE_MAX)),
            MARKDOWN_FONT_SIZE_MAX,
        );
    }

    #[test]
    fn negative_infinity_and_negative_values_clamp_to_the_minimum() {
        assert_close(
            normalize_markdown_font_size(num(f64::NEG_INFINITY)),
            MARKDOWN_FONT_SIZE_DEFAULT,
        );
        assert_close(
            normalize_markdown_font_size(num(-5.0)),
            MARKDOWN_FONT_SIZE_MIN,
        );
    }
}
