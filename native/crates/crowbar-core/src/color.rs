//! CSS colour arithmetic — the parts of it Crowbar's stylesheets actually use.
//!
//! `color-mix(in srgb, …)` is load-bearing in the React app: the file tree's
//! hover fill, its guide lines, the focus ring on a tree row and the editor's
//! overlay chrome are all defined as mixes rather than as flat colours, so a
//! port that cannot evaluate one cannot reproduce those surfaces.
//!
//! It lives here rather than in `crowbar-ui` because it is arithmetic on four
//! floats: no window, no framework, no `gpui`. §4.3 rule 1 and D2 forbid this
//! crate from ever gaining a `gpui` dependency, so [`Srgba`] is a plain sRGB
//! representation of our own and `crowbar-ui` converts at its boundary.

/// A colour in the gamma-encoded sRGB space, with **straight** (not
/// premultiplied) alpha.
///
/// Every component is `0.0..=1.0`. This is the same space CSS calls `srgb` —
/// the encoded one, not `srgb-linear` — because that is the space
/// `color-mix(in srgb, …)` interpolates in.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Srgba {
    /// Red, gamma-encoded.
    pub r: f32,
    /// Green, gamma-encoded.
    pub g: f32,
    /// Blue, gamma-encoded.
    pub b: f32,
    /// Alpha.
    pub a: f32,
}

impl Srgba {
    /// CSS's `transparent` keyword: transparent black.
    pub const TRANSPARENT: Self = Self {
        r: 0.0,
        g: 0.0,
        b: 0.0,
        a: 0.0,
    };

    /// Build a colour, clamping every component into `0.0..=1.0`.
    #[must_use]
    pub fn new(r: f32, g: f32, b: f32, a: f32) -> Self {
        Self {
            r: r.clamp(0.0, 1.0),
            g: g.clamp(0.0, 1.0),
            b: b.clamp(0.0, 1.0),
            a: a.clamp(0.0, 1.0),
        }
    }
}

/// `color-mix(in srgb, first p1%, second p2%)`.
///
/// Implements CSS Color 5 §3 exactly, which is more than a lerp:
///
/// * the two percentages are normalised so they sum to 100%, so
///   `color-mix(in srgb, a 40%, b 40%)` is the same colour as a 50/50 mix;
/// * if they summed to *less* than 100% before normalisation, the result's
///   alpha is scaled by that shortfall — this is the rule that makes
///   `color-mix(in srgb, a 30%, b 30%)` come out 60% opaque;
/// * the mix is done on **premultiplied** channels and then un-premultiplied,
///   so a translucent colour contributes in proportion to how much of it there
///   is. Mixing straight channels instead would drag the result toward the
///   hue of a colour that is barely there — a 5%-opaque black would still pull
///   a white 50% of the way to grey.
///
/// A percentage below zero is clamped to zero. If both are zero the mix is
/// undefined in CSS and the result here is [`Srgba::TRANSPARENT`]; the same
/// goes for a mix whose resulting alpha is zero, where the un-premultiply step
/// has nothing to divide by.
#[must_use]
pub fn color_mix(first: Srgba, first_percent: f32, second: Srgba, second_percent: f32) -> Srgba {
    let p1 = first_percent.max(0.0);
    let p2 = second_percent.max(0.0);
    let total = p1 + p2;
    if total <= 0.0 {
        return Srgba::TRANSPARENT;
    }

    let (w1, w2) = (p1 / total, p2 / total);
    // CSS: "if the sum is less than 100%, the alpha of the result is multiplied
    // by that sum". Above 100% it is only normalised, never scaled up.
    let shortfall = (total / 100.0).min(1.0);

    let alpha = first.a.mul_add(w1, second.a * w2);
    if alpha <= 0.0 {
        return Srgba::TRANSPARENT;
    }

    let mix = |c1: f32, c2: f32| (c1 * first.a).mul_add(w1, c2 * second.a * w2) / alpha;
    Srgba::new(
        mix(first.r, second.r),
        mix(first.g, second.g),
        mix(first.b, second.b),
        alpha * shortfall,
    )
}

/// `color-mix(in srgb, first p%, second)` — the form with the second
/// percentage omitted, which CSS defines as `100% - p`.
///
/// Every `color-mix()` in the Crowbar stylesheets is written this way.
#[must_use]
pub fn color_mix_remainder(first: Srgba, first_percent: f32, second: Srgba) -> Srgba {
    color_mix(first, first_percent, second, 100.0 - first_percent)
}

#[cfg(test)]
mod tests {
    use super::{Srgba, color_mix, color_mix_remainder};

    /// Asserts to within half an 8-bit step, which is the precision anything
    /// downstream of this can actually paint.
    fn assert_close(got: Srgba, want: Srgba) {
        let tol = 0.5 / 255.0;
        for (label, g, w) in [
            ("r", got.r, want.r),
            ("g", got.g, want.g),
            ("b", got.b, want.b),
            ("a", got.a, want.a),
        ] {
            assert!(
                (g - w).abs() < tol,
                "{label}: got {g}, want {w} (full: {got:?} vs {want:?})"
            );
        }
    }

    const WHITE: Srgba = Srgba {
        r: 1.0,
        g: 1.0,
        b: 1.0,
        a: 1.0,
    };
    const BLACK: Srgba = Srgba {
        r: 0.0,
        g: 0.0,
        b: 0.0,
        a: 1.0,
    };
    const RED: Srgba = Srgba {
        r: 1.0,
        g: 0.0,
        b: 0.0,
        a: 1.0,
    };
    const BLUE: Srgba = Srgba {
        r: 0.0,
        g: 0.0,
        b: 1.0,
        a: 1.0,
    };

    #[test]
    fn new_clamps_out_of_range_components() {
        let c = Srgba::new(-1.0, 2.0, 0.25, 7.0);
        assert_eq!(
            c,
            Srgba {
                r: 0.0,
                g: 1.0,
                b: 0.25,
                a: 1.0
            }
        );
    }

    #[test]
    fn default_is_transparent_black() {
        assert_eq!(Srgba::default(), Srgba::TRANSPARENT);
    }

    /// The mix-of-two-opaque case: half red, half blue, in the encoded space.
    #[test]
    fn mixes_two_opaque_colours() {
        assert_close(
            color_mix(RED, 50.0, BLUE, 50.0),
            Srgba {
                r: 0.5,
                g: 0.0,
                b: 0.5,
                a: 1.0,
            },
        );
    }

    /// Uneven weights on two opaque colours interpolate linearly.
    #[test]
    fn opaque_mix_respects_the_weights() {
        assert_close(
            color_mix(WHITE, 25.0, BLACK, 75.0),
            Srgba {
                r: 0.25,
                g: 0.25,
                b: 0.25,
                a: 1.0,
            },
        );
    }

    /// The mix-with-`transparent` case, which is how the whole codebase spells
    /// "this colour at N% opacity": the surviving colour keeps its channels and
    /// takes the weight as its alpha.
    #[test]
    fn mixing_with_transparent_only_scales_alpha() {
        assert_close(
            color_mix_remainder(RED, 68.0, Srgba::TRANSPARENT),
            Srgba {
                r: 1.0,
                g: 0.0,
                b: 0.0,
                a: 0.68,
            },
        );
    }

    /// `--file-tree-hover-bg` in the dark theme, end to end: `--accent` is
    /// `oklch(1 0 0 / 4%)`, so 68% of it is white at 2.72% — the value the
    /// generated theme table carries.
    #[test]
    fn reproduces_the_file_tree_hover_fill() {
        let accent = Srgba::new(1.0, 1.0, 1.0, 0.04);
        assert_close(
            color_mix_remainder(accent, 68.0, Srgba::TRANSPARENT),
            Srgba {
                r: 1.0,
                g: 1.0,
                b: 1.0,
                a: 0.0272,
            },
        );
    }

    /// `--accent 42%, --border` from the file tree's focus ring: two
    /// translucent blacks, so the result is black at the weighted alpha.
    #[test]
    fn mixes_two_translucent_colours() {
        let accent = Srgba::new(0.0, 0.0, 0.0, 0.04);
        let border = Srgba::new(0.0, 0.0, 0.0, 0.08);
        assert_close(
            color_mix_remainder(accent, 42.0, border),
            Srgba {
                r: 0.0,
                g: 0.0,
                b: 0.0,
                a: 0.04f32.mul_add(0.42, 0.08 * 0.58),
            },
        );
    }

    /// Premultiplication is the whole point: a barely-there black must barely
    /// move an opaque white. A straight-channel lerp would return mid-grey.
    #[test]
    fn premultiplication_weights_by_alpha_not_by_percentage() {
        let ghost = Srgba::new(0.0, 0.0, 0.0, 0.05);
        let got = color_mix(WHITE, 50.0, ghost, 50.0);
        assert_close(
            got,
            Srgba {
                r: 1.0 / 1.05,
                g: 1.0 / 1.05,
                b: 1.0 / 1.05,
                a: 0.525,
            },
        );
        assert!(got.r > 0.9, "a 5%-opaque black must not halve a white");
    }

    /// Percentages that sum above 100% are normalised, not scaled.
    #[test]
    fn oversized_percentages_are_normalised() {
        assert_close(
            color_mix(RED, 80.0, BLUE, 80.0),
            color_mix(RED, 50.0, BLUE, 50.0),
        );
    }

    /// Percentages that sum below 100% are normalised *and* dim the alpha.
    #[test]
    fn undersized_percentages_scale_the_alpha() {
        assert_close(
            color_mix(RED, 30.0, BLUE, 30.0),
            Srgba {
                r: 0.5,
                g: 0.0,
                b: 0.5,
                a: 0.6,
            },
        );
    }

    #[test]
    fn negative_percentages_clamp_to_zero() {
        assert_close(color_mix(RED, -20.0, BLUE, 100.0), BLUE);
    }

    #[test]
    fn two_zero_percentages_give_transparent() {
        assert_eq!(color_mix(RED, 0.0, BLUE, 0.0), Srgba::TRANSPARENT);
    }

    #[test]
    fn mixing_two_transparents_gives_transparent() {
        assert_eq!(
            color_mix(Srgba::TRANSPARENT, 50.0, Srgba::TRANSPARENT, 50.0),
            Srgba::TRANSPARENT
        );
    }
}
