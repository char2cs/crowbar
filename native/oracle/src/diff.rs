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
//!
//! # The v1.5 content-sizing model
//!
//! GPUI `ceil()`s a text run's max-content width (`elements/text.rs`:
//! `size.width = size.width.max(line_size.width).ceil()`); Blink and `WebKit`
//! keep the fraction. Measured on the gate pair, both content-sized boxes were
//! **exactly** `ceil(reference)` — `74.11 → 75`, `11.16 → 12`.
//!
//! ANCHORS.md v1.5 models that rather than widening a tolerance, and this module
//! implements the three parts:
//!
//! 1. **The target moves.** An anchor both sides declare `content_sized` has its
//!    `bounds.w` compared against `ceil(reference.w)`, keeping §5's full ±0.5
//!    around it — so a genuine sub-pixel width error is still caught, which is
//!    exactly what a looser tolerance would have given away.
//! 2. **One global allowance, no tree.** The excess is *absorbed*, not
//!    propagated: on the gate pair the flexible sibling shrank by exactly the
//!    summed excess and the trailing group's right edge was identical on both
//!    sides. So `Σ(ceil(ref.w) − ref.w)` over the declared anchors is added to
//!    the other inline measurements — one scalar off the anchor list, needing no
//!    flow order and no tree, which keeps §1's rejection of tree-diffing intact.
//! 3. **Nothing is forgiven silently.** A comparison that only passes because of
//!    the allowance, or only because the target moved, is recorded in
//!    [`Report::forgiven`] and rendered with the reason. §5 calls a silent
//!    widening the cheapest way to make a gate pass while it tells you nothing;
//!    a reader has to be able to see which findings were forgiven and why.
//!
//! ## The allowance is on the inline axis only, and that is narrower than v1.5's prose
//!
//! v1.5 says "every other geometry field in the same snapshot". Everything it
//! *measures* is horizontal: a ceiled **width**, a flexible sibling that
//! absorbed it along the same axis, a conserved right edge. There is no
//! measurement behind extending it to `bounds.y`, `bounds.h` or `radius`, and
//! §5's rule cuts the same way in both directions — an allowance applied to an
//! axis nothing was measured on is a widening with nothing behind it, which is
//! the thing the version note is at pains to avoid being.
//!
//! It is also not theoretical. On the archived gate pair the sixth delta is
//! `git-row-name.bounds.h: 19.0 vs 18.0`, a **vertical** difference with an
//! entirely different cause (GPUI snaps line height to the device grid where
//! `WebKit` floors it to a whole logical pixel). A 1.73px allowance spread over
//! every axis swallows it, and the one finding the gate has left to explain
//! disappears into slack bought by an unrelated rule.
//!
//! So: `bounds.x` and `bounds.w` get the allowance. `bounds.y`, `bounds.h`,
//! `radius` and `text_width` do not. Narrowing an allowance needs no ceremony
//! under §5; widening it back would.

use crate::color::Color;
use crate::delta::{self, Class, Delta, DeltaKind};
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

/// One anchor's contribution to the v1.5 content-sizing allowance.
#[derive(Debug, Clone, PartialEq)]
pub struct Contributor {
    /// The anchor both sides declared `content_sized`.
    pub anchor: String,
    /// `ceil(reference.bounds.w) − reference.bounds.w`, always in `[0, 1)`.
    pub excess_px: f64,
}
/// The v1.5 allowance for one comparison, and where it came from.
///
/// A value rather than a bare `f64` so a report can *show its working*: an
/// allowance whose provenance is not printed is indistinguishable from a
/// loosened tolerance, which §5 is explicit about.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct ContentSizing {
    /// `Σ(ceil(ref.w) − ref.w)` over the anchors **both** sides declared
    /// `content_sized`. Zero when there are none.
    pub excess_px: f64,
    /// The anchors that contributed a non-zero excess, in reference order.
    pub contributors: Vec<Contributor>,
}

/// Why a comparison that broke its §5 tolerance is not a delta.
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Forgiveness {
    /// The anchor is declared `content_sized`, so the expectation moved to
    /// `ceil(reference)` and the native value is within tolerance of *that*.
    Ceil {
        /// The reference's own, fractional, width.
        reference: f64,
        /// `ceil(reference)`.
        target: f64,
    },
    /// The comparison passed only once the snapshot's content-sizing allowance
    /// was added to the §5 tolerance.
    Allowance {
        /// How much slack was added, in px.
        added_px: f64,
    },
}

/// A comparison that exceeded its §5 tolerance and was forgiven anyway.
///
/// Carried separately from [`Report::deltas`] — it is not a finding — but
/// **rendered**, because a widening nobody can see is the failure §5 names.
#[derive(Debug, Clone, PartialEq)]
pub struct Forgiven {
    /// The delta it would have been, quoting the tolerance it actually broke.
    pub delta: Delta,
    /// The v1.5 rule that forgave it.
    pub reason: Forgiveness,
}

impl fmt::Display for Forgiven {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{} — forgiven: ", self.delta)?;
        match self.reason {
            Forgiveness::Ceil { reference, target } => write!(
                f,
                "this anchor is declared content_sized, so ANCHORS.md v1.5 compares it against \
                 ceil({}) = {} — GPUI ceils a text run's max-content width and cannot produce \
                 the fraction the reference kept",
                delta::px(reference),
                delta::px(target),
            ),
            Forgiveness::Allowance { added_px } => write!(
                f,
                "within tolerance once v1.5's content-sizing allowance of +{} px is added; the \
                 ceil excess on the content-sized anchors is absorbed along this axis rather \
                 than propagated",
                delta::px(round3(added_px)),
            ),
        }
    }
}

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
    /// The v1.5 content-sizing allowance in force, and the anchors it came
    /// from. `excess_px` is `0.0` when nothing was declared.
    pub content_sizing: ContentSizing,
    /// Comparisons that broke their §5 tolerance and were forgiven by a v1.5
    /// rule, ranked like the deltas.
    ///
    /// **Not** deltas: they do not change the verdict. But they are the exact
    /// list a reader needs in order to tell a modelled correction from a
    /// tolerance somebody quietly widened.
    pub forgiven: Vec<Forgiven>,
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
        // Named on the verdict line, not only in the body: a run whose headline
        // number was reached by forgiving four comparisons and one that was
        // reached by measuring them must not read the same.
        let forgiven = self.forgiven.len();
        if forgiven > 0 {
            let _ = write!(
                line,
                ", {forgiven} forgiven by v1.5 content-sizing (Σ ceil excess {} px)",
                crate::delta::px(round3(self.content_sizing.excess_px))
            );
        }
        line
    }
}

/// The contract's three decimals (v1.2 ruling 3), for **rendering only**.
///
/// A sum of three-decimal values in binary floating point lands on things like
/// `1.7300000000000004`, and twelve digits of representation noise in a
/// human-facing report is noise a reader has to learn to skip past.
///
/// The allowance itself is never rounded before it is used: rounding a
/// tolerance moves the boundary, which is a §5 loosening and is not what this
/// is.
pub(crate) fn round3(v: f64) -> f64 {
    (v * 1000.0).round() / 1000.0
}

/// `Σ(ceil(ref.w) − ref.w)` over the anchors **both** sides declare
/// `content_sized`.
///
/// Both sides, deliberately. A declaration only one extractor makes is already
/// a `FieldPresence` delta; letting it also silently buy slack for every other
/// inline measurement in the snapshot would mean a mis-declaration on one side
/// *loosened the gate*, which is the exact direction a contract defect must not
/// push. Requiring agreement makes a mis-declaration cost coverage rather than
/// hand it out.
///
/// No tree walk and no flow order: this is a fold over the anchor list, which is
/// what keeps §1's rejection of tree-diffing intact.
fn content_sizing(expected: &Snapshot, actual: &Snapshot) -> ContentSizing {
    let mut sizing = ContentSizing::default();
    for reference in &expected.anchors {
        if !reference.content_sized {
            continue;
        }
        if !actual
            .anchor(&reference.id)
            .is_some_and(|native| native.content_sized)
        {
            continue;
        }
        let excess = reference.bounds.w.ceil() - reference.bounds.w;
        sizing.excess_px += excess;
        if excess > 0.0 {
            sizing.contributors.push(Contributor {
                anchor: reference.id.clone(),
                excess_px: excess,
            });
        }
    }
    sizing
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
    let mut forgiven = Vec::new();
    let mut compared = 0_usize;
    let sizing = content_sizing(expected, actual);

    for reference in &expected.anchors {
        match actual.anchor(&reference.id) {
            Some(native) => {
                compared += 1;
                compare_anchor(
                    reference,
                    native,
                    tol,
                    sizing.excess_px,
                    &mut deltas,
                    &mut forgiven,
                );
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
    forgiven.sort_by(|a, b| a.delta.rank(&b.delta));

    Ok(Report {
        surface: expected.surface.clone(),
        state: expected.state.clone(),
        anchors_compared: compared,
        reference_anchors: expected.anchors.len(),
        native_anchors: actual.anchors.len(),
        deltas,
        content_sizing: sizing,
        forgiven,
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
    /// The snapshot-wide v1.5 allowance, in px. `0.0` when nothing declared
    /// itself content-sized, which is every snapshot written before v1.5.
    allowance: f64,
    out: &'a mut Vec<Delta>,
    forgiven: &'a mut Vec<Forgiven>,
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

    /// Records a comparison that broke `kind`'s tolerance but is not a finding.
    fn forgive(&mut self, field: &'static str, kind: DeltaKind, reason: Forgiveness) {
        self.forgiven.push(Forgiven {
            delta: Delta {
                anchor: self.id.to_owned(),
                field,
                class: Class::Geometry,
                kind,
            },
            reason,
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

    /// A measurement along the **inline** axis, which the v1.5 allowance
    /// applies to. See the module docs for why only this axis.
    fn inline_number(&mut self, field: &'static str, expected: f64, actual: f64, tolerance: f64) {
        if within(actual, expected, tolerance) {
            return;
        }
        let kind = DeltaKind::Number {
            expected,
            actual,
            tolerance,
        };
        // The line the reader sees still quotes §5's tolerance, because that is
        // the one it broke; the allowance is named in the forgiveness, where it
        // can carry its own justification. Quoting a widened tolerance instead
        // would make the report look like the contract says ±2.23.
        if self.allowance > 0.0 && within(actual, expected, tolerance + self.allowance) {
            let added_px = self.allowance;
            self.forgive(field, kind, Forgiveness::Allowance { added_px });
            return;
        }
        self.push(field, Class::Geometry, kind);
    }

    /// `bounds.w` on an anchor **both** sides declare `content_sized` (v1.5).
    ///
    /// The expectation is `ceil(reference)` and the tolerance stays §5's ±0.5,
    /// so this is strictly a *correction*: it moves the target onto the value
    /// the engine is capable of producing, and it forgives nothing else. A
    /// native box a whole pixel wider than the ceiled target is still a delta.
    fn content_sized_width(&mut self, reference: f64, actual: f64, tolerance: f64) {
        let target = reference.ceil();
        if within(actual, target, tolerance) {
            // Only worth saying when the raw comparison would have failed —
            // an integral reference width forgives nothing and reporting it
            // would train a reader to skip the section.
            if !within(actual, reference, tolerance) {
                self.forgive(
                    "bounds.w",
                    DeltaKind::Number {
                        expected: reference,
                        actual,
                        tolerance,
                    },
                    Forgiveness::Ceil { reference, target },
                );
            }
            return;
        }
        self.push(
            "bounds.w",
            Class::Geometry,
            DeltaKind::CeiledNumber {
                reference,
                expected: target,
                actual,
                tolerance,
            },
        );
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
fn compare_anchor(
    expected: &Anchor,
    actual: &Anchor,
    tol: &Tolerances,
    allowance: f64,
    out: &mut Vec<Delta>,
    forgiven: &mut Vec<Forgiven>,
) {
    let mut d = AnchorDiff {
        id: &expected.id,
        tol,
        allowance,
        out,
        forgiven,
    };
    compare_declaration(&mut d, expected, actual);
    compare_content(&mut d, expected, actual);
    compare_paint(&mut d, expected, actual);
    compare_geometry(&mut d, expected, actual);
    compare_border(&mut d, expected, actual);
    compare_typography(&mut d, expected, actual);
}

/// `content_sized`, v1.5 — the one field that is about the *contract* rather
/// than about the pixels.
///
/// It ranks [`Class::FieldPresence`] because that is what it is: a field one
/// extractor asserts and the other does not, which decides what the geometry
/// comparison below is even asking. Coercing to either side's answer would let
/// a mis-declaration open a blind spot or invent a delta and announce neither —
/// the failure mode v1.5 gives as the reason the flag is declared rather than
/// detected in the first place.
fn compare_declaration(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    if expected.content_sized != actual.content_sized {
        d.push(
            "content_sized",
            Class::FieldPresence,
            DeltaKind::Declaration {
                expected: expected.content_sized,
                actual: actual.content_sized,
            },
        );
    }
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
///
/// `x` and `w` are the inline axis and carry the v1.5 allowance; `y`, `h`,
/// `radius` and `text_width` do not. The module docs give the measurement that
/// justifies the split, and the one it would otherwise swallow.
fn compare_geometry(d: &mut AnchorDiff<'_>, expected: &Anchor, actual: &Anchor) {
    let bounds_tol = d.tol.bounds_px;
    d.inline_number("bounds.x", expected.bounds.x, actual.bounds.x, bounds_tol);
    d.number("bounds.y", expected.bounds.y, actual.bounds.y, bounds_tol);
    // The ceil rule needs *both* declarations. A disagreement is already a
    // FieldPresence delta immediately above; comparing under the plain §5 rule
    // as well means the mis-declaration costs coverage rather than buying it.
    if expected.content_sized && actual.content_sized {
        d.content_sized_width(expected.bounds.w, actual.bounds.w, bounds_tol);
    } else {
        d.inline_number("bounds.w", expected.bounds.w, actual.bounds.w, bounds_tol);
    }
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
