//! The editor settings that switch a help on or off.

use crate::settings::highlighting::Highlighting;

/// What the editor does while you type, with develop's defaults.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Assists {
    /// Suggestions open while typing, as develop's `autoCompletion`.
    pub completion: bool,
    /// The word under the caret is marked wherever it occurs, as `highlightOccurrences`.
    pub occurrences: bool,
    /// What colours the text.
    pub highlighting: Highlighting,
}

impl Default for Assists {
    fn default() -> Self {
        Self {
            completion: true,
            occurrences: false,
            highlighting: Highlighting::Semantic,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nothing_stored_helps_as_develop_does() {
        let develop = Assists {
            completion: true,
            occurrences: false,
            highlighting: Highlighting::Semantic,
        };
        assert_eq!(Assists::default(), develop);
    }
}
