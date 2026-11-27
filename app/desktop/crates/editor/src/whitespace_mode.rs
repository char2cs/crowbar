#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum WhitespaceMode {
    #[default]
    None,
    Boundary,
    Trailing,
    All,
}

pub fn marked_whitespace(
    mode: WhitespaceMode,
    line: &str,
) -> impl Iterator<Item = (usize, char)> + '_ {
    line.char_indices()
        .filter(|(_, character)| matches!(character, ' ' | '\t'))
        .filter(move |(offset, _)| mode.marks(line, *offset))
}

impl WhitespaceMode {
    fn marks(
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

    #[test]
    fn each_mode_marks_the_whitespace_monaco_renders() {
        let line = "  a b  c d ";
        let cases: [Case; 6] = [
            (WhitespaceMode::None, line, &[]),
            (WhitespaceMode::All, line, &[0, 1, 3, 5, 6, 8, 10]),
            (WhitespaceMode::Boundary, line, &[0, 1, 5, 6, 10]),
            (WhitespaceMode::Trailing, line, &[10]),
            (WhitespaceMode::Trailing, "   ", &[0, 1, 2]),
            (WhitespaceMode::Trailing, "a b \t", &[3, 4]),
        ];
        for (mode, line, expected) in cases {
            let offsets: Vec<_> = marked_whitespace(mode, line).map(|(at, _)| at).collect();
            assert_eq!(offsets, expected, "{mode:?} {line:?}");
        }
        let tab = marked_whitespace(WhitespaceMode::All, "a\t").next();
        assert_eq!(tab, Some((1, '\t')), "a tab is told from a space");
    }
}
