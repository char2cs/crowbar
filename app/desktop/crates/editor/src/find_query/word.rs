use std::ops::Range;

const SEPARATORS: &str = "`~!@#$%^&*()-=+[{]}\\|;:'\",.<>/?";

pub(crate) fn is_separator(character: char) -> bool {
    character.is_whitespace() || SEPARATORS.contains(character)
}

#[must_use]
pub fn word_at(
    text: &str,
    offset: usize,
) -> Option<Range<usize>> {
    let before = text.get(..offset)?;
    let after = text.get(offset..)?;
    let start = before
        .char_indices()
        .rev()
        .take_while(|(_, character)| !is_separator(*character))
        .last()
        .map_or(offset, |(at, _)| at);
    let length: usize = after
        .chars()
        .take_while(|character| !is_separator(*character))
        .map(char::len_utf8)
        .sum();
    let end = offset + length;
    (start < end).then_some(start..end)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_word_at_an_offset_runs_between_separators() {
        type Case = (&'static str, usize, Option<Range<usize>>);
        let cases: &[Case] = &[
            ("let total = 0;", 5, Some(4..9)),
            ("let total = 0;", 9, Some(4..9)),
            ("let total = 0;", 4, Some(4..9)),
            ("a + b", 2, None),
            ("über_x()", 3, Some(0..7)),
        ];
        for (text, offset, expected) in cases {
            assert_eq!(word_at(text, *offset), *expected, "{text:?} at {offset}");
        }
    }
}
