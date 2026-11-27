#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) enum QuoteState {
    #[default]
    Outside,
    Inside(char),
    Escaped(char),
}

const BACKSLASH: char = '\\';
const DOUBLE_QUOTE: char = '"';
const SINGLE_QUOTE: char = '\'';

impl QuoteState {
    pub(crate) fn is_outside(self) -> bool {
        self == Self::Outside
    }

    pub(crate) fn after(
        self,
        found: char,
    ) -> Self {
        match self {
            Self::Escaped(quote) => Self::Inside(quote),
            Self::Inside(DOUBLE_QUOTE) if found == BACKSLASH => Self::Escaped(DOUBLE_QUOTE),
            Self::Inside(quote) if quote == found => Self::Outside,
            Self::Outside if matches!(found, SINGLE_QUOTE | DOUBLE_QUOTE) => Self::Inside(found),
            other => other,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(QuoteState, char, QuoteState); 7] = [
        (QuoteState::Outside, '"', QuoteState::Inside('"')),
        (QuoteState::Outside, '\'', QuoteState::Inside('\'')),
        (QuoteState::Outside, 'a', QuoteState::Outside),
        (QuoteState::Inside('"'), '"', QuoteState::Outside),
        (QuoteState::Inside('"'), '\\', QuoteState::Escaped('"')),
        (QuoteState::Inside('\''), '\\', QuoteState::Inside('\'')),
        (QuoteState::Escaped('"'), '"', QuoteState::Inside('"')),
    ];

    #[test]
    fn each_character_moves_the_scan_as_a_quote_would() {
        for (before, found, after) in CASES {
            assert_eq!(before.after(found), after, "{before:?} then {found}");
        }
        assert!(QuoteState::Outside.is_outside());
        assert!(!QuoteState::Inside('"').is_outside());
    }
}
