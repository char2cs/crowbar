//! The spaces and tabs a code editor marks, as Monaco's `renderWhitespace` picks them.

mod whitespace_mode;

pub use whitespace_mode::WhitespaceMode;

/// The spaces and tabs `mode` marks in `line`, each with its byte offset.
pub fn marked_whitespace(
    mode: WhitespaceMode,
    line: &str,
) -> impl Iterator<Item = (usize, char)> + '_ {
    line.char_indices()
        .filter(|(_, character)| matches!(character, ' ' | '\t'))
        .filter(move |(offset, _)| mode.marks(line, *offset))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_mode_s_spaces_and_tabs_are_marked_and_a_tab_is_told_from_a_space() {
        let marked: Vec<_> = marked_whitespace(WhitespaceMode::Trailing, "a b \t").collect();
        assert_eq!(marked, [(3, ' '), (4, '\t')]);
    }
}
