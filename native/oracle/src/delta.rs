//! What a single disagreement is, how severe it is, and how it reads.
//!
//! # The ranking, and why it is this one
//!
//! Deltas come out sorted most-severe first. The purpose of the order is
//! narrow and worth stating: **the top of the list is where a human looks
//! first**, and a list that puts a 0.6px rounding difference above a missing
//! element wastes the only scarce resource in the loop. The classes, in order,
//! are [`Class`]:
//!
//! 1. **`AnchorPresence`** — an anchor is on one side and not the other. This
//!    is first because it is not a *difference*, it is an *absence*: there is
//!    no meaningful way to discuss `git-row-badge.bounds.x` when
//!    `git-row-badge` does not exist. It also subsumes many downstream deltas —
//!    one missing anchor suppresses the eleven field comparisons it would have
//!    produced — so surfacing it first is what stops the list from lying about
//!    how much is wrong.
//! 2. **`FieldPresence`** — the anchor exists on both sides but a field is
//!    emitted by only one. ANCHORS.md's opening rule calls this out by name: a
//!    field one extractor emits and the other does not "looks like coverage and
//!    is not". It is a defect in the *contract's implementation*, not in the
//!    UI, and it silently shrinks what the gate is checking, so it ranks above
//!    every value mismatch.
//! 3. **`Visibility`** — `visible` disagrees. The element is painted in one app
//!    and not the other. Nothing about its geometry matters until that is
//!    resolved.
//! 4. **`TextContent`** — the string differs. A user reads the wrong filename
//!    before they measure where it sits.
//! 5. **`Truncation`** — `clipped` disagrees. Separated from `TextContent`
//!    because it is the specific failure ANCHORS.md §"`text_width` and
//!    `clipped`" says the gate target was *chosen* to expose: two engines
//!    agreeing on the box and disagreeing on where the ellipsis lands. Ranking
//!    it with ordinary geometry would bury the finding the gate exists for.
//! 6. **`Colour`** — RGB is **exact** in §5, so any colour delta at all is a
//!    real defect and never engine noise. That is why it outranks geometry,
//!    where near-tolerance deltas are common, boring, and numerous.
//! 7. **`Geometry`** — bounds, `text_width`, radius, border width. Real, but
//!    graded: these have tolerances precisely because sub-pixel disagreement is
//!    expected. Sorted by magnitude, so the 40px delta is above the 0.6px one.
//! 8. **`Typography`** — font size, weight, family, line height. Last, and not
//!    because it matters least: it is the class most likely to differ for
//!    reasons that are *not* a Crowbar defect (a resolved family name spelled
//!    differently by two font stacks, platform line-height rounding). Putting
//!    the noisiest class at the top would defeat the point of ranking. A font
//!    delta that is real will normally announce itself through the geometry
//!    deltas it causes, which sit above it.
//!
//! Within a class, **larger magnitude first**, then anchor id, then field path.
//! The last two tie-breaks exist so the order is *total and stable*: two runs
//! over the same pair of snapshots must produce byte-identical output, or the
//! ranking is not something a human can learn to read.

use std::cmp::Ordering;
use std::fmt;

use crate::color::Color;

/// Severity class. Lower discriminant sorts first, i.e. more severe.
///
/// The `#[repr(u8)]` and the explicit discriminants are load-bearing: the
/// ordering *is* the discriminant order, and writing the numbers down makes an
/// accidental reordering during an edit a visible diff rather than a silent
/// change of behaviour.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
#[repr(u8)]
pub enum Class {
    /// An anchor exists on one side only.
    AnchorPresence = 0,
    /// A field is emitted by one extractor and not the other.
    FieldPresence = 1,
    /// `visible` disagrees.
    Visibility = 2,
    /// `text` disagrees.
    TextContent = 3,
    /// `clipped` disagrees.
    Truncation = 4,
    /// A colour disagrees.
    Colour = 5,
    /// A box measurement disagrees by more than its tolerance.
    Geometry = 6,
    /// A font property disagrees.
    Typography = 7,
}

impl Class {
    /// A short human label, used in the summary line.
    #[must_use]
    pub const fn label(self) -> &'static str {
        match self {
            Self::AnchorPresence => "anchor presence",
            Self::FieldPresence => "field presence",
            Self::Visibility => "visibility",
            Self::TextContent => "text",
            Self::Truncation => "truncation",
            Self::Colour => "colour",
            Self::Geometry => "geometry",
            Self::Typography => "typography",
        }
    }

    /// Every class, most severe first. The ranking, written down once.
    pub const ALL: [Self; 8] = [
        Self::AnchorPresence,
        Self::FieldPresence,
        Self::Visibility,
        Self::TextContent,
        Self::Truncation,
        Self::Colour,
        Self::Geometry,
        Self::Typography,
    ];
}

/// The specific disagreement.
#[derive(Debug, Clone, PartialEq)]
pub enum DeltaKind {
    /// The expected snapshot has this anchor; the actual one does not.
    MissingAnchor,
    /// The actual snapshot has this anchor; the expected one does not.
    UnexpectedAnchor,
    /// The expected snapshot emitted this field; the actual one did not.
    MissingField {
        /// The value the expected side had, rendered.
        expected: String,
    },
    /// The actual snapshot emitted this field; the expected one did not.
    UnexpectedField {
        /// The value the actual side had, rendered.
        actual: String,
    },
    /// An exact-match boolean disagrees.
    Bool {
        /// Reference value.
        expected: bool,
        /// Native value.
        actual: bool,
    },
    /// An exact-match string disagrees.
    Text {
        /// Reference value.
        expected: String,
        /// Native value.
        actual: String,
    },
    /// A colour disagrees, in RGB or beyond the alpha tolerance.
    Colour {
        /// Reference value.
        expected: Color,
        /// Native value.
        actual: Color,
        /// Whether the difference is in RGB (exact) or only in alpha
        /// (tolerated to ±1/255).
        rgb: bool,
        /// The alpha tolerance, quoted in the rendering when RGB agrees.
        alpha_tolerance: u8,
    },
    /// A tolerated number disagrees by more than its tolerance.
    Number {
        /// Reference value.
        expected: f64,
        /// Native value.
        actual: f64,
        /// The tolerance it broke.
        tolerance: f64,
    },
    /// An exact-match number disagrees at all.
    ///
    /// Separate from [`DeltaKind::Number`] with a zero tolerance so the
    /// rendering can say `exact` rather than quote a `±0.0` that reads like a
    /// tolerance somebody forgot to fill in. `border.w` is the one field in
    /// this class (§5, v1.1 ruling 1).
    ExactNumber {
        /// Reference value.
        expected: f64,
        /// Native value.
        actual: f64,
    },
    /// An exact-match integer disagrees.
    Integer {
        /// Reference value.
        expected: u32,
        /// Native value.
        actual: u32,
    },
    /// `bounds.w` on an anchor both sides declare `content_sized` (v1.5).
    ///
    /// The expectation is `ceil(reference)`, not the reference: GPUI ceils a
    /// text run's max-content width, so it **cannot produce** the fraction
    /// Blink kept, and comparing against that fraction asks a question the
    /// engine is incapable of answering. Both numbers are rendered, because a
    /// line that quoted only the ceiled target would not match the snapshot a
    /// reader is about to open.
    CeiledNumber {
        /// The reference's own, fractional, width.
        reference: f64,
        /// `ceil(reference)` — what the native engine can actually produce.
        expected: f64,
        /// Native value.
        actual: f64,
        /// The tolerance it broke, kept at §5's full ±0.5 around the target.
        tolerance: f64,
    },
    /// The two sides disagree on whether an anchor is `content_sized` (v1.5).
    ///
    /// Ranked [`Class::FieldPresence`] rather than coerced to either side's
    /// answer. The declaration decides which target `bounds.w` is compared
    /// against and how much slack every other inline measurement in the
    /// snapshot gets, so a mis-declaration is a defect in the contract's
    /// implementation — the class ANCHORS.md says "looks like coverage and is
    /// not" — and it must not be settled silently by whichever extractor the
    /// differ happened to read first.
    Declaration {
        /// What the reference declared.
        expected: bool,
        /// What the native side declared.
        actual: bool,
    },
}

/// One ranked disagreement between two snapshots.
#[derive(Debug, Clone, PartialEq)]
pub struct Delta {
    /// The anchor id it belongs to.
    pub anchor: String,
    /// Dotted field path within the anchor, e.g. `bounds.x`. Empty for the
    /// anchor-presence classes, which are about the anchor itself.
    pub field: &'static str,
    /// Severity class; drives the ranking.
    pub class: Class,
    /// The disagreement.
    pub kind: DeltaKind,
}

impl Delta {
    /// How wrong this delta is, in whatever unit its class measures.
    ///
    /// px for [`DeltaKind::Number`], 1/255 units for [`DeltaKind::Colour`],
    /// weight units for [`DeltaKind::Integer`], and `0.0` for the categorical
    /// kinds, whose ordering falls through to the id/field tie-break. Only ever
    /// compared against other magnitudes *within the same class*, so the mixed
    /// units never meet.
    #[must_use]
    pub fn magnitude(&self) -> f64 {
        match &self.kind {
            DeltaKind::Number {
                expected, actual, ..
            }
            | DeltaKind::ExactNumber { expected, actual }
            | DeltaKind::CeiledNumber {
                expected, actual, ..
            } => (actual - expected).abs(),
            DeltaKind::Colour {
                expected,
                actual,
                rgb,
                ..
            } => {
                let d = if *rgb {
                    actual.rgb_distance(*expected)
                } else {
                    actual.alpha_distance(*expected)
                };
                f64::from(d)
            }
            DeltaKind::Integer { expected, actual } => f64::from(actual.abs_diff(*expected)),
            DeltaKind::MissingAnchor
            | DeltaKind::UnexpectedAnchor
            | DeltaKind::MissingField { .. }
            | DeltaKind::UnexpectedField { .. }
            | DeltaKind::Bool { .. }
            | DeltaKind::Text { .. }
            | DeltaKind::Declaration { .. } => 0.0,
        }
    }

    /// The total order described in the module docs.
    ///
    /// Total and stable: class, then magnitude descending, then anchor id, then
    /// field path. `total_cmp` rather than `partial_cmp` so there is no
    /// `Option` to unwrap — magnitudes are finite by construction, since the
    /// parser rejects non-finite input.
    #[must_use]
    pub fn rank(&self, other: &Self) -> Ordering {
        self.class
            .cmp(&other.class)
            .then_with(|| other.magnitude().total_cmp(&self.magnitude()))
            .then_with(|| self.anchor.cmp(&other.anchor))
            .then_with(|| self.field.cmp(other.field))
    }

    /// `anchor.field`, or just `anchor` for an anchor-presence delta.
    fn subject(&self) -> String {
        if self.field.is_empty() {
            self.anchor.clone()
        } else {
            format!("{}.{}", self.anchor, self.field)
        }
    }
}

/// The snapshot precision ANCHORS.md v1.2 ruling 3 fixes for **both sides**:
/// three decimal places, three orders inside the ±0.5 tolerance.
const PRECISION: f64 = 1000.0;

/// A number, always with a decimal point, so `12` reads as `12.0` and a px
/// quantity never looks like a count.
///
/// Deliberately **not** rounded. These are the values the two extractors
/// actually emitted, and v1.2 already requires them to arrive at three
/// decimals; printing them verbatim is what keeps an extractor that emits more
/// precision than the contract allows visible rather than tidied away.
pub(crate) fn px(v: f64) -> String {
    format!("{v:?}")
}

/// A signed *difference*, e.g. `+4.0` or `-0.75`, rounded to the contract's
/// three decimals.
///
/// This is the one number in a delta line that nobody measured: it is
/// `actual - expected`, and subtracting two three-decimal values in binary
/// floating point lands on things like `-52.442999999999984`. Twelve digits of
/// representation noise in a human-facing report is noise a reader has to learn
/// to skip past, so the *rendering* rounds to the precision the contract says
/// the inputs have.
///
/// **The comparison is not rounded.** [`crate::tolerance::within`] sees the raw
/// `f64`s; rounding before the tolerance test would move the boundary, which is
/// a loosening under §5 and is not what this is.
fn signed(v: f64) -> String {
    let magnitude = px((v.abs() * PRECISION).round() / PRECISION);
    if v < 0.0 {
        format!("-{magnitude}")
    } else {
        format!("+{magnitude}")
    }
}

/// A signed integer difference, e.g. `+100` or `-5`.
fn signed_int(actual: u32, expected: u32) -> String {
    let d = actual.abs_diff(expected);
    if actual < expected {
        format!("-{d}")
    } else {
        format!("+{d}")
    }
}

impl fmt::Display for Delta {
    /// Renders a delta the way ANCHORS.md renders one: subject, actual,
    /// expected, magnitude, and the tolerance that was broken.
    ///
    /// ```text
    /// git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)
    /// ```
    ///
    /// Every line names the rule it broke, because "mismatch" is not something
    /// anyone can act on. Exact fields say `exact` rather than quoting a
    /// tolerance they do not have.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let subject = self.subject();
        match &self.kind {
            DeltaKind::MissingAnchor => write!(
                f,
                "{subject}: anchor missing from the native snapshot (present in the reference)"
            ),
            DeltaKind::UnexpectedAnchor => write!(
                f,
                "{subject}: anchor present in the native snapshot but not in the reference"
            ),
            DeltaKind::MissingField { expected } => write!(
                f,
                "{subject}: not emitted by the native extractor, expected {expected} \
                 (field-presence mismatch, not a value delta)"
            ),
            DeltaKind::UnexpectedField { actual } => write!(
                f,
                "{subject}: emitted by the native extractor as {actual}, absent from the \
                 reference (field-presence mismatch, not a value delta)"
            ),
            DeltaKind::Bool { expected, actual } => {
                write!(f, "{subject}: {actual}, expected {expected} (exact)")
            }
            DeltaKind::Text { expected, actual } => {
                write!(f, "{subject}: {actual:?}, expected {expected:?} (exact)")
            }
            DeltaKind::Colour {
                expected,
                actual,
                rgb,
                alpha_tolerance,
            } => {
                let (channel, diff) = actual.worst_channel(*expected);
                let sign = if diff < 0 { "-" } else { "+" };
                let d = diff.abs();
                if *rgb {
                    write!(
                        f,
                        "{subject}: {actual}, expected {expected} \
                         (Δ {channel} {sign}{d}, rgb is exact)"
                    )
                } else {
                    write!(
                        f,
                        "{subject}: {actual}, expected {expected} \
                         (Δ {channel} {sign}{d}, tol ±{alpha_tolerance}/255)"
                    )
                }
            }
            DeltaKind::Number {
                expected,
                actual,
                tolerance,
            } => write!(
                f,
                "{subject}: {}, expected {} (Δ {}, tol ±{})",
                px(*actual),
                px(*expected),
                signed(actual - expected),
                px(*tolerance)
            ),
            DeltaKind::ExactNumber { expected, actual } => write!(
                f,
                "{subject}: {}, expected {} (Δ {}, exact)",
                px(*actual),
                px(*expected),
                signed(actual - expected),
            ),
            DeltaKind::Integer { expected, actual } => write!(
                f,
                "{subject}: {actual}, expected {expected} (Δ {}, exact)",
                signed_int(*actual, *expected)
            ),
            DeltaKind::CeiledNumber {
                reference,
                expected,
                actual,
                tolerance,
            } => write!(
                f,
                "{subject}: {}, expected {} = ceil({}) (Δ {}, tol ±{}, content_sized)",
                px(*actual),
                px(*expected),
                px(*reference),
                signed(actual - expected),
                px(*tolerance)
            ),
            DeltaKind::Declaration { expected, actual } => write!(
                f,
                "{subject}: {actual}, expected {expected} (declared, exact). The two extractors \
                 disagree on whether this anchor sizes to its own text, so one of them is \
                 comparing bounds.w against the wrong target and the snapshot's content-sizing \
                 allowance is computed from the wrong set"
            ),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn color(raw: &str) -> Color {
        Color::parse(raw).expect("valid colour")
    }

    fn delta(anchor: &str, field: &'static str, class: Class, kind: DeltaKind) -> Delta {
        Delta {
            anchor: anchor.to_owned(),
            field,
            class,
            kind,
        }
    }

    #[test]
    fn renders_the_anchors_md_example_verbatim() {
        let d = delta(
            "git-row-icon",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 8.0,
                actual: 12.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(
            d.to_string(),
            "git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)"
        );
    }

    fn assert_renders(cases: &[(Delta, &str)]) {
        for (d, want) in cases {
            assert_eq!(&d.to_string(), want);
        }
    }

    #[test]
    fn presence_deltas_say_which_side_is_missing_what() {
        assert_renders(&[
            (
                delta("a", "", Class::AnchorPresence, DeltaKind::MissingAnchor),
                "a: anchor missing from the native snapshot (present in the reference)",
            ),
            (
                delta("a", "", Class::AnchorPresence, DeltaKind::UnexpectedAnchor),
                "a: anchor present in the native snapshot but not in the reference",
            ),
            (
                delta(
                    "a",
                    "text_width",
                    Class::FieldPresence,
                    DeltaKind::MissingField {
                        expected: "186.5".to_owned(),
                    },
                ),
                "a.text_width: not emitted by the native extractor, expected 186.5 \
                 (field-presence mismatch, not a value delta)",
            ),
            (
                delta(
                    "a",
                    "radius",
                    Class::FieldPresence,
                    DeltaKind::UnexpectedField {
                        actual: "2.0".to_owned(),
                    },
                ),
                "a.radius: emitted by the native extractor as 2.0, absent from the reference \
                 (field-presence mismatch, not a value delta)",
            ),
        ]);
    }

    #[test]
    fn exact_field_deltas_say_exact_rather_than_quoting_a_tolerance() {
        assert_renders(&[
            (
                delta(
                    "a",
                    "visible",
                    Class::Visibility,
                    DeltaKind::Bool {
                        expected: true,
                        actual: false,
                    },
                ),
                "a.visible: false, expected true (exact)",
            ),
            (
                delta(
                    "a",
                    "text",
                    Class::TextContent,
                    DeltaKind::Text {
                        expected: "main.rs".to_owned(),
                        actual: "main.ts".to_owned(),
                    },
                ),
                "a.text: \"main.ts\", expected \"main.rs\" (exact)",
            ),
            (
                delta(
                    "a",
                    "font.weight",
                    Class::Typography,
                    DeltaKind::Integer {
                        expected: 500,
                        actual: 400,
                    },
                ),
                "a.font.weight: 400, expected 500 (Δ -100, exact)",
            ),
        ]);
    }

    #[test]
    fn tolerated_deltas_quote_the_tolerance_they_broke() {
        assert_renders(&[
            (
                delta(
                    "a",
                    "fg",
                    Class::Colour,
                    DeltaKind::Colour {
                        expected: color("#c8ccd4ff"),
                        actual: color("#c8ccd0ff"),
                        rgb: true,
                        alpha_tolerance: 1,
                    },
                ),
                "a.fg: #c8ccd0ff, expected #c8ccd4ff (Δ b -4, rgb is exact)",
            ),
            (
                delta(
                    "a",
                    "bg",
                    Class::Colour,
                    DeltaKind::Colour {
                        expected: color("#00000000"),
                        actual: color("#00000005"),
                        rgb: false,
                        alpha_tolerance: 1,
                    },
                ),
                "a.bg: #00000005, expected #00000000 (Δ a +5, tol ±1/255)",
            ),
            (
                delta(
                    "a",
                    "bounds.y",
                    Class::Geometry,
                    DeltaKind::Number {
                        expected: 4.0,
                        actual: 3.25,
                        tolerance: 0.5,
                    },
                ),
                "a.bounds.y: 3.25, expected 4.0 (Δ -0.75, tol ±0.5)",
            ),
        ]);
    }

    /// v1.5. The ceiled comparison names **both** numbers: the target the
    /// engine can meet and the reference's own fraction, so the line still
    /// matches the snapshot a reader is about to open.
    #[test]
    fn the_content_sized_kinds_say_what_rule_they_are_under() {
        assert_renders(&[
            (
                delta(
                    "git-row-badge",
                    "bounds.w",
                    Class::Geometry,
                    DeltaKind::CeiledNumber {
                        reference: 74.11,
                        expected: 75.0,
                        actual: 80.0,
                        tolerance: 0.5,
                    },
                ),
                "git-row-badge.bounds.w: 80.0, expected 75.0 = ceil(74.11) \
                 (Δ +5.0, tol ±0.5, content_sized)",
            ),
            (
                delta(
                    "git-row-badge",
                    "content_sized",
                    Class::FieldPresence,
                    DeltaKind::Declaration {
                        expected: true,
                        actual: false,
                    },
                ),
                "git-row-badge.content_sized: false, expected true (declared, exact). \
                 The two extractors disagree on whether this anchor sizes to its own text, \
                 so one of them is comparing bounds.w against the wrong target and the \
                 snapshot's content-sizing allowance is computed from the wrong set",
            ),
        ]);
    }

    #[test]
    fn magnitude_measures_each_kind_in_its_own_unit() {
        let number = delta(
            "a",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 8.0,
                actual: 12.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(number.magnitude().to_bits(), 4.0_f64.to_bits());

        let exact = delta(
            "a",
            "border.w",
            Class::Geometry,
            DeltaKind::ExactNumber {
                expected: 1.0,
                actual: 1.5,
            },
        );
        assert_eq!(exact.magnitude().to_bits(), 0.5_f64.to_bits());

        let rgb = delta(
            "a",
            "fg",
            Class::Colour,
            DeltaKind::Colour {
                expected: color("#000000ff"),
                actual: color("#0a0000ff"),
                rgb: true,
                alpha_tolerance: 1,
            },
        );
        assert_eq!(rgb.magnitude().to_bits(), 10.0_f64.to_bits());

        let alpha = delta(
            "a",
            "bg",
            Class::Colour,
            DeltaKind::Colour {
                expected: color("#00000000"),
                actual: color("#00000007"),
                rgb: false,
                alpha_tolerance: 1,
            },
        );
        assert_eq!(alpha.magnitude().to_bits(), 7.0_f64.to_bits());

        let weight = delta(
            "a",
            "font.weight",
            Class::Typography,
            DeltaKind::Integer {
                expected: 500,
                actual: 400,
            },
        );
        assert_eq!(weight.magnitude().to_bits(), 100.0_f64.to_bits());

        // v1.5: measured against the **ceiled** target, not the reference's
        // fraction, so the ranking sorts by the error the engine could have
        // avoided rather than by the one it cannot.
        let ceiled = delta(
            "a",
            "bounds.w",
            Class::Geometry,
            DeltaKind::CeiledNumber {
                reference: 74.11,
                expected: 75.0,
                actual: 80.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(ceiled.magnitude().to_bits(), 5.0_f64.to_bits());

        for categorical in [
            DeltaKind::Declaration {
                expected: true,
                actual: false,
            },
            DeltaKind::MissingAnchor,
            DeltaKind::UnexpectedAnchor,
            DeltaKind::MissingField {
                expected: "x".to_owned(),
            },
            DeltaKind::UnexpectedField {
                actual: "x".to_owned(),
            },
            DeltaKind::Bool {
                expected: true,
                actual: false,
            },
            DeltaKind::Text {
                expected: "a".to_owned(),
                actual: "b".to_owned(),
            },
        ] {
            let d = delta("a", "f", Class::Visibility, categorical);
            assert_eq!(d.magnitude().to_bits(), 0.0_f64.to_bits());
        }
    }

    #[test]
    fn rank_orders_by_class_then_magnitude_then_id_then_field() {
        let severe = delta("z", "", Class::AnchorPresence, DeltaKind::MissingAnchor);
        let mild = delta(
            "a",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 40.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(severe.rank(&mild), Ordering::Less);
        assert_eq!(mild.rank(&severe), Ordering::Greater);

        let smaller = delta(
            "a",
            "bounds.y",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 1.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(
            mild.rank(&smaller),
            Ordering::Less,
            "larger magnitude first"
        );

        let same_a = delta(
            "aaa",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 1.0,
                tolerance: 0.5,
            },
        );
        let same_b = delta(
            "bbb",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 1.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(
            same_a.rank(&same_b),
            Ordering::Less,
            "anchor id breaks ties"
        );

        let field_x = delta(
            "a",
            "bounds.x",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 1.0,
                tolerance: 0.5,
            },
        );
        let field_y = delta(
            "a",
            "bounds.y",
            Class::Geometry,
            DeltaKind::Number {
                expected: 0.0,
                actual: 1.0,
                tolerance: 0.5,
            },
        );
        assert_eq!(field_x.rank(&field_y), Ordering::Less, "field breaks ties");
        assert_eq!(field_x.rank(&field_x.clone()), Ordering::Equal);
    }

    #[test]
    fn the_class_order_is_the_documented_one() {
        let ordered: Vec<&str> = Class::ALL.iter().map(|c| c.label()).collect();
        assert_eq!(
            ordered,
            vec![
                "anchor presence",
                "field presence",
                "visibility",
                "text",
                "truncation",
                "colour",
                "geometry",
                "typography",
            ]
        );
        // `ALL` is in ascending severity order, which is what the sort relies on.
        let mut sorted = Class::ALL;
        sorted.sort_unstable();
        assert_eq!(sorted, Class::ALL);
    }

    #[test]
    fn number_formatting_never_hides_the_decimal_or_the_sign() {
        assert_eq!(px(12.0), "12.0");
        assert_eq!(px(0.5), "0.5");
        assert_eq!(px(186.53), "186.53");
        assert_eq!(signed(4.0), "+4.0");
        assert_eq!(signed(-0.75), "-0.75");
        assert_eq!(signed(0.0), "+0.0");
        assert_eq!(signed_int(400, 500), "-100");
        assert_eq!(signed_int(500, 400), "+100");
        assert_eq!(signed_int(400, 400), "+0");
    }
}
