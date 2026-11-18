use std::ops::Range;

use gpui::{WrappedLine, px};

use crate::control::editor::text_area::text_area_layout::text_row::TextRow;
use crate::foundation::Pose;

pub(crate) fn break_index(
    wrapped: &WrappedLine,
    boundary: &gpui::WrapBoundary,
) -> usize {
    wrapped
        .runs()
        .get(boundary.run_ix)
        .and_then(|run| run.glyphs.get(boundary.glyph_ix))
        .map_or(0, |glyph| glyph.index)
}

pub(crate) fn caret_before(
    wrapped: &WrappedLine,
    end: usize,
) -> usize {
    wrapped
        .text
        .get(..end)
        .and_then(|head| head.chars().next_back())
        .map_or(end, |last| end - last.len_utf8())
}

pub(crate) fn caret_edge(
    wrapped: &WrappedLine,
    to: usize,
    closes_line: Pose,
) -> usize {
    if closes_line == Pose::On {
        return to;
    }
    caret_before(wrapped, to)
}

pub(crate) fn text_row(
    wrapped: &WrappedLine,
    start: usize,
    span: Range<usize>,
    closes: Pose,
) -> TextRow {
    TextRow {
        bytes: start + span.start..start + span.end,
        caret_end: start + caret_edge(wrapped, span.end, closes),
        top: px(0.0),
        origin: wrapped.unwrapped_layout.x_for_index(span.start),
        line: 0,
        closes_line: closes == Pose::On,
    }
}

pub(crate) fn span_of(pair: &[usize]) -> Range<usize> {
    pair.first().copied().unwrap_or(0)..pair.last().copied().unwrap_or(0)
}

pub(crate) fn edges_of(wrapped: &WrappedLine) -> Vec<usize> {
    let breaks = wrapped
        .wrap_boundaries()
        .iter()
        .map(|boundary| break_index(wrapped, boundary));
    std::iter::once(0)
        .chain(breaks)
        .chain(std::iter::once(wrapped.len()))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static [usize], Range<usize>);

    #[test]
    fn a_span_runs_from_the_first_edge_to_the_last() {
        let cases: [Case; 4] = [
            (&[], 0..0),
            (&[3], 3..3),
            (&[0, 4], 0..4),
            (&[2, 5, 9], 2..9),
        ];
        for (edges, span) in cases {
            assert_eq!(span_of(edges), span, "{edges:?}");
        }
    }
}
