/// Which spaces and tabs a code editor marks, as Monaco's `renderWhitespace`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum WhitespaceMode {
    /// No markers.
    #[default]
    None,
    /// Every space and tab except a single space between two words.
    Boundary,
    /// The spaces and tabs after the last other character of a line.
    Trailing,
    /// Every space and tab.
    All,
}

impl WhitespaceMode {
    pub(super) fn marks(
        self,
        line: &str,
        offset: usize,
    ) -> bool {
        match self {
            Self::None => false,
            Self::All => true,
            Self::Trailing => offset >= line.trim_end_matches([' ', '\t']).len(),
            Self::Boundary => !is_lone_space_between_words(line, offset),
        }
    }
}

fn is_lone_space_between_words(
    line: &str,
    offset: usize,
) -> bool {
    let is_blank = |character: char| matches!(character, ' ' | '\t');
    let (Some(before), Some(rest)) = (line.get(..offset), line.get(offset..)) else {
        return false;
    };
    let previous = before.chars().next_back();
    let next = rest.chars().nth(1);
    rest.starts_with(' ')
        && previous.is_some_and(|character| !is_blank(character))
        && next.is_some_and(|character| !is_blank(character))
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (WhitespaceMode, &'static str, &'static [usize]);

    fn offsets(
        mode: WhitespaceMode,
        line: &str,
    ) -> Vec<usize> {
        line.char_indices()
            .filter(|(offset, character)| *character == ' ' && mode.marks(line, *offset))
            .map(|(offset, _)| offset)
            .collect()
    }

    #[test]
    fn each_mode_marks_the_whitespace_monaco_renders() {
        let line = "  a b  c d ";
        let cases: [Case; 5] = [
            (WhitespaceMode::None, line, &[]),
            (WhitespaceMode::All, line, &[0, 1, 3, 5, 6, 8, 10]),
            (WhitespaceMode::Boundary, line, &[0, 1, 5, 6, 10]),
            (WhitespaceMode::Trailing, line, &[10]),
            (WhitespaceMode::Trailing, "   ", &[0, 1, 2]),
        ];
        for (mode, line, expected) in cases {
            assert_eq!(offsets(mode, line), expected, "{mode:?} {line:?}");
        }
    }
}
