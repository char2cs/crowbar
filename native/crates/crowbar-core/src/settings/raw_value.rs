//! A minimal stand-in for TypeScript's `unknown`, scoped to exactly the two
//! shapes this area's clamp functions actually branch on.
//!
//! `lib/ui-font-size.ts`'s `normalizeUiFontSize(value: unknown)` and
//! `lib/markdown-font-size.ts`'s `normalizeMarkdownFontSize(value: unknown)`
//! both open with the same reduction: `typeof value === 'number' ? value :
//! typeof value === 'string' ? Number(value) : NaN`. That is the entire
//! surface `unknown` is used for in this area — a genuine JS number, a
//! string that might parse as one (a persisted value that round-tripped
//! through `JSON.stringify`/a form input), or anything else, which
//! collapses to the same "not usable" fallback regardless of whether it was
//! `null`, `undefined`, an object, or an array. There is no ported function
//! anywhere in this crate that needs to keep those "anything else" shapes
//! distinguishable, so [`RawNumber`] has exactly two variants and "anything
//! else" is represented by passing `None` for the `Option<RawNumber>`
//! itself, one layer up — see [`super::ui_font_size::normalize_ui_font_size`]
//! and [`super::markdown_font_size::normalize_markdown_font_size`].
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum RawNumber<'a> {
    /// `typeof value === 'number'`.
    Number(f64),
    /// `typeof value === 'string'` — still needs `Number(value)` and an
    /// `isFinite` check downstream; a non-numeric string parses to `NaN`
    /// exactly like a genuinely absent value does.
    Text(&'a str),
}

impl RawNumber<'_> {
    /// `typeof value === 'number' ? value : typeof value === 'string' ?
    /// Number(value) : NaN`, minus the final `NaN` arm (that's `None` one
    /// layer up).
    ///
    /// Not bit-for-bit `Number(str)`: JS's `Number('')`/`Number('   ')`
    /// coerce to `0`, where `"".trim().parse::<f64>()` is an `Err` here and
    /// therefore `NaN`. Neither ported test suite
    /// (`ui-font-size.test.ts`/`markdown-font-size.test.ts`) exercises an
    /// empty-string input, so this divergence is not covered by a ported
    /// test — noted rather than silently matched, per this port's own
    /// standard for behaviour that wasn't checked.
    #[must_use]
    pub fn as_f64(self) -> f64 {
        match self {
            RawNumber::Number(n) => n,
            RawNumber::Text(s) => s.trim().parse().unwrap_or(f64::NAN),
        }
    }
}
