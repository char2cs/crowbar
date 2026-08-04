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
//!
//! [`parse_css_color`] is the other half of the same problem: `theme.css`
//! authors its colours as `#hex`, `rgb()`/`rgba()` and (mostly) `oklch()`
//! strings, so evaluating a token means parsing CSS syntax before there is
//! anything to mix. Ported from `resolve-css-color.ts`'s pure region —
//! `cssColorToHex`/`oklchToHex`/`gammaEncode`/`expandShortHex`/`parseAlpha`
//! — per `native/mapping/tier-a-denominator.md`'s theme-tokens section; see
//! `native/mapping/core-color-parse.md` for the full port record, including
//! what was deliberately left as a webview-only concern (`resolveCssVar` and
//! friends, which exist only because a browser cannot read a CSS custom
//! property back out of `getComputedStyle()`).

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

// ---------------------------------------------------------------------------
// CSS colour *parsing* — `resolve-css-color.ts`'s pure region.
// ---------------------------------------------------------------------------

/// Parses a CSS colour string into an [`Srgba`].
///
/// Ported from `resolve-css-color.ts`'s `cssColorToHex`. Recognises the
/// formats `theme.css` actually authors colours in: `#rgb`, `#rrggbb`,
/// `#rrggbbaa`, `rgb()`/`rgba()` (comma- or space-separated), and `oklch()`.
/// Returns `None` for empty or unrecognised input — the TS source's `null`.
///
/// The TS function returns a `#rrggbb[aa]` *string*, because that is the
/// shape Monaco/xterm's DOM APIs want it in. There is no such consumer here
/// — `crowbar-ui` converts at its boundary, the same way it already does for
/// [`color_mix_remainder`] — so this returns [`Srgba`] directly, which is a
/// strictly more precise representation than the source: the TS pipeline
/// quantises every channel through an 8-bit hex byte (`toHexByte`/
/// `clamp255`, not ported — there is nothing left for them to quantise
/// into), so e.g. `rgba(0, 0, 0, 0.5)` comes back out of `cssColorToHex` as
/// `#00000080` (alpha 128/255 ≈ 0.50196), where this returns alpha `0.5`
/// exactly. The ported tests assert against the exact value, not the
/// TS source's lossy one.
#[must_use]
pub fn parse_css_color(value: &str) -> Option<Srgba> {
    let v = value.trim().to_ascii_lowercase();
    if v.is_empty() {
        return None;
    }

    if let Some(hex) = v.strip_prefix('#') {
        return parse_hex(hex);
    }
    if let Some(inner) = v.strip_prefix("rgba(").or_else(|| v.strip_prefix("rgb(")) {
        return parse_rgb_functional(inner.strip_suffix(')')?);
    }
    if let Some(inner) = v.strip_prefix("oklch(") {
        return parse_oklch_functional(inner.strip_suffix(')')?);
    }

    None
}

/// `[\d.]+` — the character class the TS regex used for every numeric
/// component: digits and a decimal point, no sign, no exponent. Matching it
/// explicitly (rather than accepting anything `f32`'s `FromStr` does) keeps
/// this exactly as strict as the source, not more lenient.
fn parse_decimal(s: &str) -> Option<f32> {
    if s.is_empty() || !s.bytes().all(|b| b.is_ascii_digit() || b == b'.') {
        return None;
    }
    s.parse().ok()
}

/// `#rgb` / `#rrggbb` / `#rrggbbaa` (already lower-cased, `#` already
/// stripped). The short (`#rgb`) form doubles each digit, matching
/// `expandShortHex`; there is no 4-digit `#rgba` shorthand in the source and
/// this does not invent one. Rejecting non-hex-digit input here, before any
/// byte-range slicing of the (possibly-doubled) string, is what makes that
/// slicing safe — every character left standing is a single-byte ASCII hex
/// digit.
fn parse_hex(hex: &str) -> Option<Srgba> {
    if !hex.bytes().all(|b| b.is_ascii_hexdigit()) {
        return None;
    }
    let expanded: String = match hex.len() {
        3 => hex.chars().flat_map(|ch| [ch, ch]).collect(),
        6 | 8 => hex.to_string(),
        _ => return None,
    };
    let r = u8::from_str_radix(&expanded[0..2], 16).ok()?;
    let g = u8::from_str_radix(&expanded[2..4], 16).ok()?;
    let b = u8::from_str_radix(&expanded[4..6], 16).ok()?;
    let a = if expanded.len() == 8 {
        u8::from_str_radix(&expanded[6..8], 16).ok()?
    } else {
        255
    };
    Some(Srgba::new(
        f32::from(r) / 255.0,
        f32::from(g) / 255.0,
        f32::from(b) / 255.0,
        f32::from(a) / 255.0,
    ))
}

/// A parsed alpha token: a bare `[\d.]+` float, or a `[\d.]+%` percentage
/// divided by 100. Matches `parseAlpha`, restricted to the callers here
/// always passing a non-empty token (never TS's `undefined` case — every
/// call site below handles "alpha omitted" itself, matching the source's
/// `raw == null` branch, before ever reaching this function).
fn parse_alpha(raw: &str) -> Option<f32> {
    match raw.strip_suffix('%') {
        Some(pct) => parse_decimal(pct).map(|p| p / 100.0),
        None => parse_decimal(raw),
    }
}

/// The inside of `rgb(...)`/`rgba(...)`: three `[\d.]+` components,
/// comma- or space-separated, with an optional trailing alpha after a comma
/// or `/`. Anchored the way the source's regex was — trailing tokens past
/// alpha are rejected, not ignored.
fn parse_rgb_functional(inner: &str) -> Option<Srgba> {
    let normalized: String = inner
        .chars()
        .map(|ch| if ch == ',' || ch == '/' { ' ' } else { ch })
        .collect();
    let mut tokens = normalized.split_whitespace();
    let r = parse_decimal(tokens.next()?)?;
    let g = parse_decimal(tokens.next()?)?;
    let b = parse_decimal(tokens.next()?)?;
    let alpha = match tokens.next() {
        Some(raw) => parse_alpha(raw)?,
        None => 1.0,
    };
    if tokens.next().is_some() {
        return None;
    }
    Some(Srgba::new(r / 255.0, g / 255.0, b / 255.0, alpha))
}

/// The inside of `oklch(...)`: `L C H`, space-separated (never
/// comma-separated — the source's regex never allowed that for this
/// function, unlike `rgb()`), `L` optionally a percentage, with an optional
/// trailing alpha after `/`.
fn parse_oklch_functional(inner: &str) -> Option<Srgba> {
    let normalized: String = inner
        .chars()
        .map(|ch| if ch == '/' { ' ' } else { ch })
        .collect();
    let mut tokens = normalized.split_whitespace();
    let l_raw = tokens.next()?;
    let l = match l_raw.strip_suffix('%') {
        Some(pct) => parse_decimal(pct)? / 100.0,
        None => parse_decimal(l_raw)?,
    };
    let c = parse_decimal(tokens.next()?)?;
    let h_deg = parse_decimal(tokens.next()?)?;
    let alpha = match tokens.next() {
        Some(raw) => parse_alpha(raw)?,
        None => 1.0,
    };
    if tokens.next().is_some() {
        return None;
    }
    Some(oklch_to_srgb(l, c, h_deg, alpha))
}

/// `oklch(l c h)` → gamma-encoded [`Srgba`]. Matrices per Björn Ottosson's
/// `OKLab` reference, ported from `oklchToHex`. The same matrices appear a
/// second time in this codebase, independently, in
/// `crowbar-ui/tools/gen-theme.py`'s own `oklch_to_srgb` (the Python-side
/// theme-generation step, which does not depend on this crate at all); that
/// script's `check_palette()` proves them against Tailwind's own published
/// hex values, and one of its fixtures is reused below as an extra,
/// non-TS-sourced check on this implementation
/// (`tests::cross_checks_a_tailwind_swatch_against_gen_theme_pys_fixture`).
fn oklch_to_srgb(l: f32, c: f32, h_deg: f32, alpha: f32) -> Srgba {
    let hue = h_deg.to_radians();
    let a = c * hue.cos();
    let b = c * hue.sin();

    let l2 = l + 0.396_337_78 * a + 0.215_803_76 * b;
    let m2 = l - 0.105_561_346 * a - 0.063_854_17 * b;
    let s2 = l - 0.089_484_18 * a - 1.291_485_5 * b;

    let l3 = l2.powi(3);
    let m3 = m2.powi(3);
    let s3 = s2.powi(3);

    let red = 4.076_741_7 * l3 - 3.307_711_6 * m3 + 0.230_969_94 * s3;
    let green = -1.268_438 * l3 + 2.609_757_4 * m3 - 0.341_319_38 * s3;
    let blue = -0.004_196_086_3 * l3 - 0.703_418_6 * m3 + 1.707_614_7 * s3;

    Srgba::new(
        gamma_encode(red),
        gamma_encode(green),
        gamma_encode(blue),
        alpha,
    )
}

/// Linear-light channel → gamma-encoded sRGB channel (the sRGB transfer
/// function), ported from `gammaEncode`. [`Srgba::new`] does the
/// `0.0..=1.0` clamp `clamp255` did in the source; there is no `* 255` step
/// because [`Srgba`] stores continuous floats, not hex bytes.
fn gamma_encode(c: f32) -> f32 {
    if c <= 0.003_130_8 {
        12.92 * c
    } else {
        1.055 * c.powf(1.0 / 2.4) - 0.055
    }
}

#[cfg(test)]
mod tests {
    use super::{Srgba, color_mix, color_mix_remainder, oklch_to_srgb, parse_css_color};

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

    // -----------------------------------------------------------------
    // `parse_css_color` — ported from `resolve-css-color.test.ts`'s
    // `describe('cssColorToHex', …)` block, 8/8 cases, 1:1. That file's other
    // `describe('DOM resolver', …)` block (5 cases) tests
    // `resolveCssVar`/`readSyntaxPalette`/`readTerminalPalette`, which were
    // not ported — see `native/mapping/core-color-parse.md` for why.
    // -----------------------------------------------------------------

    /// Ported: `'passes through and expands hex'`.
    #[test]
    fn hex_passes_through_and_expands_shorthand() {
        assert_close(
            parse_css_color("#aabbcc").expect("6-digit hex parses"),
            Srgba::new(170.0 / 255.0, 187.0 / 255.0, 204.0 / 255.0, 1.0),
        );
        assert_close(
            parse_css_color("#abc").expect("3-digit hex parses"),
            Srgba::new(170.0 / 255.0, 187.0 / 255.0, 204.0 / 255.0, 1.0),
        );
        assert_close(
            parse_css_color("  #ABCDEF ").expect("trims and lower-cases first"),
            Srgba::new(171.0 / 255.0, 205.0 / 255.0, 239.0 / 255.0, 1.0),
        );
    }

    /// Ported: `'converts rgb/rgba'`.
    #[test]
    fn converts_rgb_and_rgba() {
        assert_close(
            parse_css_color("rgb(255, 0, 0)").expect("rgb parses"),
            Srgba::new(1.0, 0.0, 0.0, 1.0),
        );
        assert_close(
            parse_css_color("rgba(0, 0, 0, 0.5)").expect("rgba parses"),
            Srgba::new(0.0, 0.0, 0.0, 0.5),
        );
    }

    /// Ported: `'converts oklch endpoints exactly'`.
    #[test]
    fn converts_oklch_endpoints_exactly() {
        assert_close(
            parse_css_color("oklch(1 0 0)").expect("oklch parses"),
            Srgba::new(1.0, 1.0, 1.0, 1.0),
        );
        assert_close(
            parse_css_color("oklch(0 0 0)").expect("oklch parses"),
            Srgba::new(0.0, 0.0, 0.0, 1.0),
        );
    }

    /// Ported: `'handles oklch alpha'`.
    #[test]
    fn handles_oklch_alpha() {
        assert_close(
            parse_css_color("oklch(1 0 0 / 50%)").expect("oklch with alpha parses"),
            Srgba::new(1.0, 1.0, 1.0, 0.5),
        );
    }

    /// Ported: `'converts a known chromatic oklch within tolerance of sRGB
    /// red'`. The TS test asserts on individual 8-bit channel bytes
    /// (`r > 250`, `g`/`b < 20`, out of 255); these are the same thresholds
    /// expressed as the `0.0..=1.0` fractions this returns instead.
    #[test]
    fn converts_a_known_chromatic_oklch_within_tolerance_of_srgb_red() {
        let got = parse_css_color("oklch(0.6279 0.2577 29.23)").expect("oklch parses");
        assert!(got.r > 250.0 / 255.0, "strong red channel: {got:?}");
        assert!(got.g < 20.0 / 255.0, "green near zero: {got:?}");
        assert!(got.b < 20.0 / 255.0, "blue near zero: {got:?}");
    }

    /// Ported: `'passes through 8-digit hex (rrggbbaa)'`.
    #[test]
    fn passes_through_eight_digit_hex() {
        assert_close(
            parse_css_color("#aabbccdd").expect("8-digit hex parses"),
            Srgba::new(170.0 / 255.0, 187.0 / 255.0, 204.0 / 255.0, 221.0 / 255.0),
        );
    }

    /// Ported: `'converts space-separated rgb'`.
    #[test]
    fn converts_space_separated_rgb() {
        assert_close(
            parse_css_color("rgb(255 0 0)").expect("space-separated rgb parses"),
            Srgba::new(1.0, 0.0, 0.0, 1.0),
        );
    }

    /// Ported: `'returns null for unparseable input'`.
    #[test]
    fn returns_none_for_unparseable_input() {
        assert_eq!(parse_css_color(""), None);
        assert_eq!(parse_css_color("not-a-color"), None);
    }

    // -----------------------------------------------------------------
    // Authored — not in the TS suite. `resolve-css-color.test.ts` never
    // exercises `parse_hex`'s bad-digit/wrong-length rejections or either
    // functional form's trailing-token rejection, because the TS source
    // reaches all of those the same way: one anchored regex either matches
    // the whole string or it does not. This hand-written parser reaches them
    // as separate branches, each one written here explicitly.
    // -----------------------------------------------------------------

    /// Six ways to fail to parse, one per branch: wrong hex length, non-hex
    /// digits, a trailing token after `rgb()`'s alpha, a missing `oklch()`
    /// component, an unparseable `oklch()` alpha, and a trailing token after
    /// `oklch()`'s alpha.
    #[test]
    fn rejects_malformed_input_in_every_recognised_prefix() {
        assert_eq!(parse_css_color("#12345"), None, "wrong hex length (5)");
        assert_eq!(parse_css_color("#zzzzzz"), None, "non-hex digits");
        assert_eq!(
            parse_css_color("rgb(255 0 0 0.5 1)"),
            None,
            "trailing token past rgb()'s alpha"
        );
        assert_eq!(parse_css_color("oklch(1 0)"), None, "missing hue component");
        assert_eq!(
            parse_css_color("oklch(1 0 0 / nope)"),
            None,
            "unparseable alpha"
        );
        assert_eq!(
            parse_css_color("oklch(1 0 0 0.5 0.5)"),
            None,
            "trailing token past oklch()'s alpha"
        );
    }

    /// Cross-checks `oklch_to_srgb` directly against
    /// `crowbar-ui/tools/gen-theme.py`'s own, independently-authored
    /// `oklch_to_srgb` (Python, not generated from or by this file) — at
    /// nine points spanning low/mid/high lightness *and* low/mid/high
    /// chroma, deliberately chosen so **no channel at any point sits near 0
    /// or 255**.
    ///
    /// That constraint is the point, not decoration: an earlier version of
    /// this test used a single highly-saturated Tailwind swatch
    /// (`red-500`), and a mutation sweep found its red channel saturates to
    /// `1.0` — a wrong `OKLab` coefficient has to be wrong by several percent
    /// before the pre-clamp value drops back under the saturation
    /// threshold, so the test was blind to smaller, more realistic
    /// transcription errors by construction. See
    /// `native/mapping/core-color-parse.md` §7 for the measured floor
    /// before and after this test was rewritten.
    ///
    /// The expected values are `gen-theme.py`'s `oklch_to_srgb` called
    /// directly and printed at full `f64` precision — the function's raw,
    /// **un-clamped, un-quantised** output, not a hex byte read back off a
    /// published swatch. Comparing against a hex byte (as
    /// `check_palette()` itself does, and as this test used to) caps
    /// detection at roughly 1 part in 255 no matter how many swatches are
    /// added, because the reference value itself is only 8-bit precise.
    /// Comparing continuous floats has no such floor; `EPSILON` below is
    /// chosen from the actual observed `f32`-vs-`f64` computation noise
    /// (see the comment on it), not from a hex byte's resolution.
    #[test]
    fn cross_checks_nine_swatches_against_gen_theme_pys_oklch_to_srgb() {
        /// `f32` arithmetic through nine multiplications and three
        /// `powi(3)`/`powf` calls disagrees with the `f64` reference by at
        /// most `2.98e-7` across these nine swatches on the unmutated
        /// implementation (measured directly: every one of the 27
        /// channel-diffs printed with `eprintln!` before this epsilon was
        /// chosen; the worst was `L=0.75 C=0.14 H=150 r`). `1e-5` is over
        /// 33x that observed noise ceiling — comfortable headroom against a
        /// false failure from ordinary `f32` rounding — and still ~200x
        /// tighter than `assert_close`'s `0.5/255 ≈ 2e-3` used elsewhere in
        /// this file, which is itself already tighter than any hex-byte
        /// comparison could be. See `native/mapping/core-color-parse.md` §7
        /// for the mutation sweep that measures what this actually buys in
        /// coefficient-error terms.
        const EPSILON: f32 = 1e-5;

        // (L, C, H, expected r, g, b) — `l`/`c`/`h` chosen so every channel
        // lands comfortably mid-range (55–225 of 255) at all nine points;
        // expected values are `gen-theme.py`'s `oklch_to_srgb(l, c, h)`,
        // called directly, `repr()`-precision.
        let swatches: [(f32, f32, f32, f32, f32, f32); 9] = [
            // low L
            (0.35, 0.03, 270.0, 0.204_982_86, 0.227_327_4, 0.292_316_05),
            (0.35, 0.08, 270.0, 0.166_372_77, 0.216_090_97, 0.392_132_74),
            (0.35, 0.14, 270.0, 0.127_840_81, 0.183_375_31, 0.509_270_4),
            // mid L
            (0.55, 0.03, 270.0, 0.417_737_1, 0.443_506_9, 0.516_406_36),
            (0.55, 0.08, 270.0, 0.373_410_37, 0.435_923_33, 0.630_422),
            (0.55, 0.14, 30.0, 0.708_765_7, 0.296_092_73, 0.240_072_94),
            // high L
            (0.75, 0.03, 150.0, 0.632_078_44, 0.704_428_9, 0.643_566_11),
            (0.75, 0.08, 150.0, 0.539_243_1, 0.739_878_6, 0.577_67),
            (0.75, 0.14, 150.0, 0.396_147, 0.778_219_9, 0.491_561_76),
        ];

        for (l, c, h, want_r, want_g, want_b) in swatches {
            let got = oklch_to_srgb(l, c, h, 1.0);
            for (label, g, w) in [
                ("r", got.r, want_r),
                ("g", got.g, want_g),
                ("b", got.b, want_b),
            ] {
                let diff = (g - w).abs();
                assert!(
                    diff < EPSILON,
                    "L={l} C={c} H={h} {label}: got {g}, want {w} (diff {diff}, epsilon {EPSILON})"
                );
            }
        }
    }
}
