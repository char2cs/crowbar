//! The comparison: two snapshots in, ranked deltas out.
//!
//! Two rules shape everything here.
//!
//! **It refuses before it compares.** ANCHORS.md §2: a delta between two
//! different states is meaningless, and comparing across states "would be the
//! easiest possible way to fake convergence". So `schema`, `surface` and every
//! field of `state` must agree, and a difference in any of them is a
//! [`ContextMismatch`] — an error the caller has to handle, not a delta it can
//! sort to the bottom and ignore.
//!
//! **It matches anchors by `id` only.** §1: the DOM and GPUI element trees are
//! not isomorphic, so there is no correspondence between their *shapes*; the id
//! is the entire correspondence function. Order is irrelevant, nesting is
//! irrelevant, and an anchor on one side only is a delta of its own kind rather
//! than a wrongly-paired comparison.

use crate::color::Color;
use crate::delta::{Class, Delta, DeltaKind};
use crate::snapshot::{Anchor, Border, Font, Snapshot, State};
use crate::tolerance::{Tolerances, within};

use std::fmt::{self, Write as _};

/// One field of the comparison context that disagreed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ContextField {
    /// The field, e.g. `state.theme`.
    pub field: &'static str,
    /// The reference snapshot's value, rendered.
    pub expected: String,
    /// The native snapshot's value, rendered.
    pub actual: String,
}

/// The two snapshots are not the same matrix cell, so they cannot be compared.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ContextMismatch {
    /// Every field that disagreed, in a fixed order.
    pub fields: Vec<ContextField>,
}

impl fmt::Display for ContextMismatch {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut s = String::from(
            "refusing to compare: the two snapshots are not the same §8.3 matrix cell. \
             A delta between different states is meaningless, and comparing across them is \
             the easiest possible way to fake convergence (ANCHORS.md §2).",
        );
        for field in &self.fields {
            let _ = write!(
                s,
                "\n  {}: native {}, reference {}",
                field.field, field.actual, field.expected
            );
        }
        f.write_str(&s)
    }
}

impl std::error::Error for ContextMismatch {}

/// The result of comparing two snapshots.
#[derive(Debug, Clone, PartialEq)]
pub struct Report {
    /// The surface both snapshots named.
    pub surface: String,
    /// The matrix cell both snapshots were taken in.
    pub state: State,
    /// Anchors present in **both** snapshots — the number actually compared.
    ///
    /// Reported rather than inferred, because "0 anchors, 0 deltas" is a pass
    /// on paper and tells you nothing at all. Callers are expected to look.
    pub anchors_compared: usize,
    /// How many anchors the reference snapshot carried.
    pub reference_anchors: usize,
    /// How many anchors the native snapshot carried.
    pub native_anchors: usize,
    /// Every disagreement, ranked most severe first.
    pub deltas: Vec<Delta>,
}

impl Report {
    /// Whether anything at all exceeded tolerance.
    #[must_use]
    pub fn is_clean(&self) -> bool {
        self.deltas.is_empty()
    }

    /// How many deltas fall in each class, most severe class first. Classes
    /// with no deltas are omitted.
    #[must_use]
    pub fn by_class(&self) -> Vec<(Class, usize)> {
        Class::ALL
            .into_iter()
            .filter_map(|class| {
                let n = self.deltas.iter().filter(|d| d.class == class).count();
                (n > 0).then_some((class, n))
            })
            .collect()
    }

    /// The one-line verdict, e.g.
    /// `FAIL — 3 deltas over 9 anchors compared (1 anchor presence, 2 geometry)`.
    #[must_use]
    pub fn summary(&self) -> String {
        let verdict = if self.is_clean() { "PASS" } else { "FAIL" };
        let n = self.deltas.len();
        let plural = if n == 1 { "" } else { "s" };
        let mut line = format!(
            "{verdict} — {n} delta{plural} over {} anchor{} compared",
            self.anchors_compared,
            if self.anchors_compared == 1 { "" } else { "s" }
        );
        let classes = self.by_class();
        if !classes.is_empty() {
            let parts: Vec<String> = classes
                .iter()
                .map(|(class, count)| format!("{count} {}", class.label()))
                .collect();
            let _ = write!(line, " ({})", parts.join(", "));
        }
        line
    }
}

/// Compare a reference snapshot against a native one.
///
/// `expected` is the reference side (the React app), `actual` is the native
/// side. The rendering says "expected" for the former throughout, so getting
/// the two the wrong way round produces deltas whose signs are backwards but
/// whose count is identical — the argument names are the only guard and they
/// are deliberately not symmetric.
///
/// # Errors
///
/// [`ContextMismatch`] if `schema`, `surface` or any field of `state` differs.
/// That is a refusal, not a delta: see the module docs.
pub fn diff(
    expected: &Snapshot,
    actual: &Snapshot,
    tol: &Tolerances,
) -> Result<Report, ContextMismatch> {
    let mismatches = context_mismatches(expected, actual);
    if !mismatches.is_empty() {
        return Err(ContextMismatch { fields: mismatches });
    }

    let mut deltas = Vec::new();
    let mut compared = 0_usize;

    for reference in &expected.anchors {
        match actual.anchor(&reference.id) {
            Some(native) => {
                compared += 1;
                compare_anchor(reference, native, tol, &mut deltas);
            }
            None => deltas.push(Delta {
                anchor: reference.id.clone(),
                field: "",
                class: Class::AnchorPresence,
                kind: DeltaKind::MissingAnchor,
            }),
        }
    }
    for native in &actual.anchors {
        if expected.anchor(&native.id).is_none() {
            deltas.push(Delta {
                anchor: native.id.clone(),
                field: "",
                class: Class::AnchorPresence,
                kind: DeltaKind::UnexpectedAnchor,
            });
        }
    }

    deltas.sort_by(Delta::rank);

    Ok(Report {
        surface: expected.surface.clone(),
        state: expected.state.clone(),
        anchors_compared: compared,
        reference_anchors: expected.anchors.len(),
        native_anchors: actual.anchors.len(),
        deltas,
    })
}

/// Every context field that disagrees, in a fixed order so the message is
/// stable.
fn context_mismatches(expected: &Snapshot, actual: &Snapshot) -> Vec<ContextField> {
    let mut out = Vec::new();
    let mut check = |field: &'static str, e: String, a: String| {
        if e != a {
            out.push(ContextField {
                field,
                expected: e,
                actual: a,
            });
        }
    };
    check(
        "schema",
        expected.schema.to_string(),
        actual.schema.to_string(),
    );
    check("surface", expected.surface.clone(), actual.surface.clone());
    check(
        "state.width",
        expected.state.width.to_string(),
        actual.state.width.to_string(),
    );
    check(
        "state.theme",
        expected.state.theme.clone(),
        actual.state.theme.clone(),
    );
    check(
        "state.content",
        expected.state.content.clone(),
        actual.state.content.clone(),
    );
    check(
        "state.flags",
        format!("[{}]", expected.state.flags.join(", ")),
        format!("[{}]", actual.state.flags.join(", ")),
    );
    out
}

/// Builder for the per-field comparisons of one anchor.
struct AnchorDiff<'a> {
    id: &'a str,
    tol: &'a Tolerances,
    out: &'a mut Vec<Delta>,
}

impl AnchorDiff<'_> {
    fn push(&mut self, field: &'static str, class: Class, kind: DeltaKind) {
        self.out.push(Delta {
            anchor: self.id.to_owned(),
            field,
            class,
            kind,
        });
    }

    fn number(&mut self, field: &'static str, expected: f64, actual: f64, tolerance: f64) {
        if !within(actual, expected, tolerance) {
            self.push(
                field,
                Class::Geometry,
                DeltaKind::Number {
                    expected,
                    actual,
                    tolerance,
                },
            );
        }
    }

    /// A §5 **exact** number: any difference at all is a delta. Renders as
    /// `exact` rather than quoting a `±0.0` that reads like an unfilled knob.
    fn exact_number(&mut self, field: &'static str, expected: f64, actual: f64) {
        if !within(actual, expected, 0.0) {
            self.push(
                field,
                Class::Geometry,
                DeltaKind::ExactNumber { expected, actual },
            );
        }
    }

    fn typography_number(
        &mut self,
        field: &'static str,
        expected: f64,
        actual: f64,
        tolerance: f64,
    ) {
        if !within(actual, expected, tolerance) {
            self.push(
                field,
                Class::Typography,
                DeltaKind::Number {
                    expected,
                    actual,
                    tolerance,
                },
            );
        }
    }

    /// §5: RGB exact, alpha ±1/255. An RGB difference is reported as an RGB
    /// difference even if alpha is also off, because RGB is the exact rule and
    /// the one worth naming.
    fn color(&mut self, field: &'static str, expected: Color, actual: Color) {
        let rgb_off = actual.rgb_distance(expected) > 0;
        let alpha_off = actual.alpha_distance(expected) > self.tol.alpha;
        if rgb_off || alpha_off {
            let alpha_tolerance = self.tol.alpha;
            self.push(
                field,
                Class::Colour,
                DeltaKind::Colour {
                    expected,
                    actual,
                    rgb: rgb_off,
                    alpha_tolerance,
                },
            );
        }
    }

    /// An optional field that only one side emitted. Returns `true` when the
    /// presence itself was the delta, so the caller skips the value compare.
    fn presence<T>(
        &mut self,
        field: &'static str,
        expected: Option<&T>,
        actual: Option<&T>,
        render: impl Fn(&T) -> String,
    ) -> bool {
        match (expected, actual) {
            (Some(e), None) => {
                self.push(
                    field,
                    Class::FieldPresence,
                    DeltaKind::MissingField {
                        expected: render(e),
                    },
                );
                true
            }
            (None, Some(a)) => {
                self.push(
                    field,
                    Class::FieldPresence,
                    DeltaKind::UnexpectedField { actual: render(a) },
                );
                true
            }
            _ => false,
        }
    }
}

/// Every field comparison for one anchor, in severity order so the
/// pre-sort list already reads roughly right.
///
/// The five groups are **independent**, which is v1.4's clarification stated as
/// code: an anchor may be both a painted box and a text run — the gate target's
/// badge is a rounded, tinted, bordered box whose content is the word
/// `uncommitted` — and each group is compared if and only if it is present.
/// Nothing here makes the box fields and the text group exclusive, and nothing
/// may.
fn compare_anchor(expected: &Anchor, actual: &Anchor, tol: &Tolerances, out: &mut Vec<Delta>) {
    let mut d = AnchorDiff {
        id: &expected.id,
        tol,
        out,
    };
    compare_content(&mut d, expected, actual);
    compare_paint(&mut d, expected, actual);
    compare_geometry(&mut d, expected, actual);
    compare_border(&mut d, expected, actual);
    compare_typography(&mut d, expected, actual);
}

/// `visible`, `text`, `clipped` — the exact-match facts about what the user
/// sees before they measure anything.
fn compare_content(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    if expected.visible != actual.visible {
        d.push(
            "visible",
            Class::Visibility,
            DeltaKind::Bool {
                expected: expected.visible,
                actual: actual.visible,
            },
        );
    }

    if !d.presence("text", expected.text.as_ref(), actual.text.as_ref(), |s| {
        format!("{s:?}")
    }) && let (Some(e), Some(a)) = (&expected.text, &actual.text)
        && e != a
    {
        d.push(
            "text",
            Class::TextContent,
            DeltaKind::Text {
                expected: e.clone(),
                actual: a.clone(),
            },
        );
    }

    if !d.presence(
        "clipped",
        expected.clipped.as_ref(),
        actual.clipped.as_ref(),
        std::string::ToString::to_string,
    ) && let (Some(e), Some(a)) = (expected.clipped, actual.clipped)
        && e != a
    {
        d.push(
            "clipped",
            Class::Truncation,
            DeltaKind::Bool {
                expected: e,
                actual: a,
            },
        );
    }
}

/// `fg` and `bg`. `bg` is required on every anchor; `fg` only paints when the
/// anchor paints text.
fn compare_paint(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    if !d.presence(
        "fg",
        expected.fg.as_ref(),
        actual.fg.as_ref(),
        Color::to_string,
    ) && let (Some(e), Some(a)) = (expected.fg, actual.fg)
    {
        d.color("fg", e, a);
    }
    d.color("bg", expected.bg, actual.bg);
}

/// `bounds`, `text_width` and `radius` — the tolerated px quantities.
fn compare_geometry(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    let bounds_tol = d.tol.bounds_px;
    d.number("bounds.x", expected.bounds.x, actual.bounds.x, bounds_tol);
    d.number("bounds.y", expected.bounds.y, actual.bounds.y, bounds_tol);
    d.number("bounds.w", expected.bounds.w, actual.bounds.w, bounds_tol);
    d.number("bounds.h", expected.bounds.h, actual.bounds.h, bounds_tol);

    if !d.presence(
        "text_width",
        expected.text_width.as_ref(),
        actual.text_width.as_ref(),
        |v| format!("{v:?}"),
    ) && let (Some(e), Some(a)) = (expected.text_width, actual.text_width)
    {
        d.number("text_width", e, a, d.tol.text_width_px);
    }

    if !d.presence(
        "radius",
        expected.radius.as_ref(),
        actual.radius.as_ref(),
        |v| format!("{v:?}"),
    ) && let (Some(e), Some(a)) = (expected.radius, actual.radius)
    {
        d.number("radius", e, a, d.tol.radius_px);
    }
}

/// The border, whose two halves land in different classes: the width is
/// geometry, the colour is colour.
///
/// # `border.color` is compared only when the border is painted (v1.3 ruling 2)
///
/// A zero-width border has no colour anybody can see, and neither engine
/// reports a *meaningful* one for it: `getComputedStyle` falls back to the
/// element's inherited **text** colour, so the DOM side reports things like
/// `#f5f5f5ff` for a border that does not exist, while GPUI reports its own
/// default. Comparing those produced eight deltas across eight anchors on the
/// first real gate run, every one of them about paint that never happened.
///
/// **Both** widths are checked, not just the reference's. The contract says the
/// differ ignores the colour "below that threshold", and the value is junk on
/// whichever side reports zero — a real colour on one side against junk on the
/// other is the same false alarm in the other direction. Nothing is swallowed
/// by this: a width that disagrees is reported by `border.w` immediately above,
/// and §5 makes that one **exact**.
fn compare_border(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    if !d.presence(
        "border",
        expected.border.as_ref(),
        actual.border.as_ref(),
        render_border,
    ) && let (Some(e), Some(a)) = (expected.border, actual.border)
    {
        d.exact_number("border.w", e.w, a.w);
        if e.w > 0.0 && a.w > 0.0 {
            d.color("border.color", e.color, a.color);
        }
    }
}

/// `font`, all four sub-fields. Size and line height are tolerated; weight and
/// family are exact.
fn compare_typography(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    if d.presence(
        "font",
        expected.font.as_ref(),
        actual.font.as_ref(),
        render_font,
    ) {
        return;
    }
    let (Some(e), Some(a)) = (&expected.font, &actual.font) else {
        return;
    };
    d.typography_number("font.size", e.size, a.size, d.tol.font_size_px);
    d.typography_number(
        "font.line_height",
        e.line_height,
        a.line_height,
        d.tol.line_height_px,
    );
    if e.weight != a.weight {
        d.push(
            "font.weight",
            Class::Typography,
            DeltaKind::Integer {
                expected: e.weight,
                actual: a.weight,
            },
        );
    }
    if e.family != a.family {
        d.push(
            "font.family",
            Class::Typography,
            DeltaKind::Text {
                expected: e.family.clone(),
                actual: a.family.clone(),
            },
        );
    }
}

fn render_border(b: &Border) -> String {
    format!("{{ w: {:?}, color: {} }}", b.w, b.color)
}

fn render_font(f: &Font) -> String {
    format!(
        "{{ size: {:?}, weight: {}, family: {:?}, line_height: {:?} }}",
        f.size, f.weight, f.family, f.line_height
    )
}
