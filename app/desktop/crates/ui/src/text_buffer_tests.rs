use super::*;

#[test]
fn typing_replaces_the_selection_and_moves_the_caret() {
    let mut buffer = TextBuffer::with_text("hello world");
    buffer.move_to(0);
    buffer.select_to(5);
    buffer.replace(None, "bye");
    assert_eq!(buffer.text(), "bye world");
    assert_eq!(buffer.selection(), 3..3);
}

#[test]
fn selection_direction_follows_the_anchor() {
    let mut buffer = TextBuffer::with_text("abcdef");
    buffer.move_to(3);
    buffer.select_to(1);
    assert_eq!((buffer.selection(), buffer.is_reversed(), buffer.cursor()), (1..3, true, 1));
    buffer.select_to(5);
    assert_eq!((buffer.selection(), buffer.is_reversed(), buffer.cursor()), (3..5, false, 5));
    assert_eq!(buffer.selected_text(), "de");
}

#[test]
fn boundaries_respect_graphemes() {
    let buffer = TextBuffer::with_text("a\u{1F468}\u{200D}\u{1F469}b");
    let family_end = buffer.next_boundary(1);
    assert_eq!(&buffer.text()[1..family_end], "\u{1F468}\u{200D}\u{1F469}");
    assert_eq!(buffer.previous_boundary(family_end), 1);
    assert_eq!(buffer.next_boundary(buffer.text().len()), buffer.text().len());
    assert_eq!(buffer.previous_boundary(0), 0);
}

#[test]
fn utf16_offsets_round_trip() {
    let buffer = TextBuffer::with_text("a\u{1F600}\u{e9}z");
    for (utf8, utf16) in [(0, 0), (1, 1), (5, 3), (7, 4), (8, 5)] {
        assert_eq!(buffer.to_utf16(utf8), utf16, "to {utf8}");
        assert_eq!(buffer.from_utf16(utf16), utf8, "from {utf16}");
    }
}

#[test]
fn composition_is_replaced_until_committed() {
    let mut buffer = TextBuffer::with_text("");
    buffer.replace_and_mark(None, "ni", None);
    assert_eq!(buffer.marked(), Some(0..2));
    buffer.replace_and_mark(None, "\u{4f60}", None);
    assert_eq!((buffer.text(), buffer.marked()), ("\u{4f60}", Some(0..3)));
    buffer.replace(None, "\u{4f60}");
    assert_eq!((buffer.text(), buffer.marked(), buffer.selection()), ("\u{4f60}", None, 3..3));
}

#[test]
fn out_of_range_offsets_are_clamped_to_char_boundaries() {
    let mut buffer = TextBuffer::with_text("\u{e9}a");
    buffer.move_to(1);
    assert_eq!(buffer.cursor(), 0);
    buffer.replace(Some(0..99), "x");
    assert_eq!(buffer.text(), "x");
}

#[test]
fn select_all_covers_the_text() {
    let mut buffer = TextBuffer::with_text("abc");
    buffer.select_all();
    assert_eq!(buffer.selected_text(), "abc");
}
