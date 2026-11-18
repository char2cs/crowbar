//! A stretch of consecutive cells shaped as one piece of text.

use crate::snapshot::row::style_span::StyleSpan;

/// Text that starts at `column` and advances one cell per character (two for a wide glyph).
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Segment {
    /// First column of the text.
    pub column: usize,
    /// Cells the text occupies.
    pub columns: usize,
    /// Cells each glyph spans: 1, or 2 for a wide glyph.
    pub glyph_columns: usize,
    /// The characters, zero-width marks included.
    pub text: String,
    /// Styles, in order, covering `text` end to end.
    pub spans: Vec<StyleSpan>,
}

impl Segment {
    /// Removes plain trailing spaces; true when nothing is left.
    pub fn trim_blanks(&mut self) -> bool {
        while self.text.ends_with(' ') && self.last_span_is_plain() {
            self.text.pop();
            self.columns -= 1;
            self.shrink_last_span();
        }
        self.text.is_empty()
    }

    fn last_span_is_plain(&self) -> bool {
        self.spans.last().is_some_and(|span| {
            span.underline == crate::snapshot::row::underline::Underline::None
                && !span
                    .flags
                    .contains(crate::snapshot::row::style_flags::StyleFlags::STRIKE)
        })
    }

    fn shrink_last_span(&mut self) {
        let Some(span) = self.spans.last_mut() else {
            return;
        };
        span.len -= 1;
        if span.len == 0 {
            self.spans.pop();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::snapshot::palette::Rgb;
    use crate::snapshot::row::style_flags::StyleFlags;
    use crate::snapshot::row::underline::Underline;

    fn segment(
        text: &str,
        underline: Underline,
    ) -> Segment {
        let span = StyleSpan {
            len: text.len(),
            foreground: Rgb::from_hex(0),
            flags: StyleFlags::NONE,
            underline,
        };
        Segment {
            column: 0,
            columns: text.len(),
            glyph_columns: 1,
            text: text.to_owned(),
            spans: vec![span],
        }
    }

    type Case = (&'static str, Underline, bool, &'static str, usize);

    #[test]
    fn trimming_follows_the_styles() {
        const CASES: [Case; 3] = [
            ("ab  ", Underline::None, false, "ab", 2),
            ("ab  ", Underline::Single, false, "ab  ", 4),
            ("   ", Underline::None, true, "", 0),
        ];
        for (text, underline, empty, kept, spans_len) in CASES {
            let mut value = segment(text, underline);
            assert_eq!(value.trim_blanks(), empty, "{text:?}");
            let covered: usize = value.spans.iter().map(|span| span.len).sum();
            assert_eq!(
                (value.text.as_str(), covered),
                (kept, spans_len),
                "{text:?}"
            );
        }
    }
}
