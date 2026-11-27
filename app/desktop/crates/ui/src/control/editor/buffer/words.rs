use std::ops::Range;

use unicode_segmentation::UnicodeSegmentation;

pub(crate) fn previous_start(
    text: &str,
    offset: usize,
) -> usize {
    text.unicode_word_indices()
        .rev()
        .find_map(|(start, _)| (start < offset).then_some(start))
        .unwrap_or(0)
}

pub(crate) fn next_end(
    text: &str,
    offset: usize,
) -> usize {
    text.unicode_word_indices()
        .find_map(|(start, word)| (start + word.len() > offset).then_some(start + word.len()))
        .unwrap_or(text.len())
}

pub(crate) fn around(
    text: &str,
    offset: usize,
) -> Range<usize> {
    let segments: Vec<Range<usize>> = text
        .split_word_bound_indices()
        .map(|(start, segment)| start..start + segment.len())
        .collect();
    let hit = segments.iter().find(|span| offset < span.end);
    hit.or(segments.last()).cloned().unwrap_or(0..0)
}

pub(crate) fn line_around(
    text: &str,
    offset: usize,
) -> Range<usize> {
    let head = text.get(..offset).unwrap_or(text);
    let tail = text.get(offset..).unwrap_or_default();
    let start = head.rfind('\n').map_or(0, |at| at + 1);
    let end = tail.find('\n').map_or(text.len(), |at| offset + at);
    start..end
}

#[cfg(test)]
mod tests {
    use super::*;

    const TEXT: &str = "foo  bar-baz\nqux";

    #[test]
    fn word_movement_lands_on_word_edges() {
        type Case = (usize, usize, usize);
        const CASES: &[Case] = &[
            (0, 0, 3),
            (3, 0, 8),
            (4, 0, 8),
            (5, 0, 8),
            (8, 5, 12),
            (9, 5, 12),
            (12, 9, 16),
            (13, 9, 16),
            (16, 13, 16),
        ];
        for &(offset, back, forward) in CASES {
            assert_eq!(previous_start(TEXT, offset), back, "back from {offset}");
            assert_eq!(next_end(TEXT, offset), forward, "forward from {offset}");
        }
    }

    #[test]
    fn a_double_click_takes_the_word_or_the_run_between_words() {
        type Case = (usize, Range<usize>);
        let cases: &[Case] = &[
            (0, 0..3),
            (2, 0..3),
            (3, 3..5),
            (6, 5..8),
            (8, 8..9),
            (16, 13..16),
        ];
        for (offset, want) in cases {
            assert_eq!(around(TEXT, *offset), *want, "at {offset}");
        }
        assert_eq!(around("", 0), 0..0);
    }

    #[test]
    fn a_triple_click_takes_the_line() {
        type Case = (usize, Range<usize>);
        let cases: &[Case] = &[(0, 0..12), (12, 0..12), (13, 13..16), (16, 13..16)];
        for (offset, want) in cases {
            assert_eq!(line_around(TEXT, *offset), *want, "at {offset}");
        }
    }
}
