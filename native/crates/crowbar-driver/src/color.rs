//! `Hsla` → `#rrggbbaa`, the conversion the colour half of the oracle rests on.
//!
//! `native/oracle/ANCHORS.md` §5 compares colour channels **exactly**, so an
//! off-by-one here does not show up as a delta on one anchor — it shows up on
//! *every* anchor, and reads like a theming bug rather than a rounding bug.
//! Hence its own module and its own tests.
//!
//! Note what is deliberately **not** reused: gpui's own `impl From<Rgba> for
//! u32` quantises with `(channel * 255.0) as u32`, which truncates. A channel
//! that comes back from the HSL round trip as `0.99999994` becomes `254`, not
//! `255`. The float→float half of the conversion is gpui's, so we report the
//! colour gpui actually paints; the float→`u8` half is ours, and it rounds.

use crowbar_ui::gpui::{Hsla, Rgba};

/// The contract's spelling of "this element paints nothing here".
pub(crate) const TRANSPARENT: &str = "#00000000";

/// Formats a gpui colour as the contract's `#rrggbbaa` sRGB hex string.
pub(crate) fn hex(color: Hsla) -> String {
    let rgba = Rgba::from(color);
    format!(
        "#{:02x}{:02x}{:02x}{:02x}",
        quantise(rgba.r),
        quantise(rgba.g),
        quantise(rgba.b),
        quantise(rgba.a),
    )
}

/// Maps one 0.0–1.0 channel onto 0–255, rounding to nearest.
#[expect(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "the clamp puts the value in 0.0..=255.0 before the cast, so \
              neither truncation nor a sign change is reachable"
)]
fn quantise(channel: f32) -> u8 {
    (channel.clamp(0.0, 1.0) * 255.0).round() as u8
}

#[cfg(test)]
mod tests {
    use crowbar_ui::gpui::{Hsla, black, rgb, rgba, transparent_black, white};

    use super::{TRANSPARENT, hex, quantise};

    /// The path a real theme colour takes: authored as a hex literal, stored by
    /// gpui as `Hsla`, read back by the extractor.
    fn round_trip(packed: u32) -> String {
        hex(Hsla::from(rgb(packed)))
    }

    #[test]
    fn the_transparent_constant_is_what_the_converter_produces() {
        assert_eq!(hex(transparent_black()), TRANSPARENT);
    }

    #[test]
    fn black_and_white_survive_the_round_trip() {
        assert_eq!(hex(black()), "#000000ff");
        assert_eq!(hex(white()), "#ffffffff");
    }

    #[test]
    fn the_contracts_own_example_colour_round_trips() {
        // `native/oracle/ANCHORS.md` §3 uses `#c8ccd4ff` as its sample `fg`.
        assert_eq!(round_trip(0x00c8_ccd4), "#c8ccd4ff");
    }

    #[test]
    fn primaries_and_secondaries_round_trip() {
        for (packed, expected) in [
            (0x00ff_0000_u32, "#ff0000ff"),
            (0x0000_ff00, "#00ff00ff"),
            (0x0000_00ff, "#0000ffff"),
            (0x00ff_ff00, "#ffff00ff"),
            (0x0000_ffff, "#00ffffff"),
            (0x00ff_00ff, "#ff00ffff"),
        ] {
            assert_eq!(round_trip(packed), expected, "for {packed:#010x}");
        }
    }

    #[test]
    fn alpha_is_carried_through_and_rounded_to_the_nearest_step() {
        assert_eq!(hex(Hsla::from(rgba(0x1122_3380))), "#11223380");
        assert_eq!(hex(Hsla::from(rgba(0x1122_3301))), "#11223301");
        assert_eq!(hex(Hsla::from(rgba(0x1122_33fe))), "#112233fe");
    }

    /// Greys are where a truncating quantiser shows up loudest: `r == g == b`,
    /// so the error lands on all three channels at once.
    #[test]
    fn every_grey_level_survives_the_round_trip() {
        for level in 0u32..=255 {
            let packed = (level << 16) | (level << 8) | level;
            assert_eq!(
                round_trip(packed),
                format!("#{level:02x}{level:02x}{level:02x}ff"),
                "grey level {level}",
            );
        }
    }

    /// A stride over the whole cube. Exhaustive would be 16.7M conversions;
    /// every 17th value covers all 16 nibble-aligned steps per channel, which
    /// is where a rounding error surfaces first.
    #[test]
    fn a_stride_over_the_whole_rgb_cube_survives_the_round_trip() {
        for r in (0u32..=255).step_by(17) {
            for g in (0u32..=255).step_by(17) {
                for b in (0u32..=255).step_by(17) {
                    let packed = (r << 16) | (g << 8) | b;
                    assert_eq!(
                        round_trip(packed),
                        format!("#{r:02x}{g:02x}{b:02x}ff"),
                        "for {packed:#08x}",
                    );
                }
            }
        }
    }

    /// The specific defect this module exists to avoid. gpui's `as u32` would
    /// answer 254 to the first of these and 199 to the second.
    #[test]
    fn the_quantiser_rounds_rather_than_truncates() {
        assert_eq!(quantise(0.999_999_94), 255);
        assert_eq!(quantise(0.784_313_5), 200);
    }

    #[test]
    fn out_of_range_channels_clamp_rather_than_wrap() {
        assert_eq!(quantise(-1.0), 0);
        assert_eq!(quantise(2.0), 255);
        assert_eq!(quantise(f32::NAN), 0);
    }

    #[test]
    fn a_colour_outside_the_srgb_gamut_clamps_to_the_edge() {
        // Lightness above 1.0 is representable as `Hsla` and is not a colour;
        // gpui clamps it at paint time and so do we.
        let over = Hsla {
            h: 0.0,
            s: 1.0,
            l: 2.0,
            a: 1.0,
        };

        assert_eq!(hex(over), "#ffffffff");
    }
}
