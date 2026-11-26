//! Replacing an editor's whole text as one undo step that keeps its carets and scroll, as
//! Monaco's `pushEditOperations` does for develop's reloads and its other views' edits.

use std::ops::Range;

use gpui::{Context, Window};
use gpui_kit::component::input::EditorState;

pub(crate) fn replace_keeping_selections(
    state: &mut EditorState,
    text: &str,
    window: &mut Window,
    cx: &mut Context<'_, EditorState>,
) {
    let old = state.value().to_string();
    if old == text {
        return;
    }
    let span = changed_span(&old, text);
    let selected = state.selected_range();
    let reversed = state.is_selection_reversed();
    let scroll = state.scroll_offset();
    let Some(inserted) = text.get(span.start..text.len() - (old.len() - span.end)) else {
        return;
    };
    state.set_selected_range(span.clone(), cx);
    state.replace(inserted.to_owned(), window, cx);
    let moved = |offset| text.floor_char_boundary(moved_offset(offset, &span, inserted.len()));
    let kept = moved(selected.start)..moved(selected.end);
    state.set_selected_range_with_direction(kept, reversed, cx);
    state.set_scroll_offset(scroll, cx);
}

fn changed_span(
    old: &str,
    new: &str,
) -> Range<usize> {
    let prefix = common_len(old.char_indices(), new.chars());
    let longest_suffix = old.len().min(new.len()) - prefix;
    let (old_rest, new_rest) = (
        old.get(prefix..).unwrap_or(""),
        new.get(prefix..).unwrap_or(""),
    );
    let suffix = common_len(old_rest.char_indices().rev(), new_rest.chars().rev());
    let suffix = suffix.min(longest_suffix);
    prefix..old.len() - suffix
}

fn common_len(
    old: impl Iterator<Item = (usize, char)>,
    new: impl Iterator<Item = char>,
) -> usize {
    old.zip(new)
        .take_while(|((_, a), b)| a == b)
        .map(|((_, a), _)| a.len_utf8())
        .sum()
}

fn moved_offset(
    offset: usize,
    span: &Range<usize>,
    inserted: usize,
) -> usize {
    if offset <= span.start {
        return offset;
    }
    if offset >= span.end {
        return offset - span.end + span.start + inserted;
    }
    offset.min(span.start + inserted)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_changed_span_lies_between_the_common_prefix_and_suffix() {
        let cases = [
            ("one\ntwo\n", "one\nTWO\n", 4..7),
            ("aaa", "aaaa", 3..3),
            ("abc", "abc!", 3..3),
            ("é", "è", 0..2),
            ("ab", "", 0..2),
        ];
        for (old, new, span) in cases {
            assert_eq!(changed_span(old, new), span, "{old:?} -> {new:?}");
        }
    }

    #[test]
    fn carets_before_the_span_stay_inside_it_keep_their_place_and_after_it_move_with_the_text() {
        let span = 4..7;
        let cases = [
            ("before", 2, 5, 2),
            ("after", 8, 5, 10),
            ("inside a longer insert", 5, 5, 5),
            ("inside a shorter insert", 6, 1, 5),
        ];
        for (case, offset, inserted, expected) in cases {
            assert_eq!(moved_offset(offset, &span, inserted), expected, "{case}");
        }
    }
}
