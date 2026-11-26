//! One line of text under a record's title.

use crate::records::tone::Tone;

/// A line of text with its tone and family.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Line {
    pub(crate) text: String,
    pub(crate) mono: bool,
    pub(crate) tone: Tone,
}

impl Line {
    /// The tone the text is coloured in.
    #[must_use]
    pub fn tone(&self) -> Tone {
        self.tone
    }

    /// The text shown.
    #[must_use]
    pub fn text(&self) -> &str {
        &self.text
    }

    /// Whether the text uses the monospace family.
    #[must_use]
    pub fn is_mono(&self) -> bool {
        self.mono
    }

    /// A muted line of plain text.
    pub fn plain(text: impl Into<String>) -> Self {
        Self {
            text: text.into(),
            mono: false,
            tone: Tone::Neutral,
        }
    }

    /// The same line in the monospace family.
    #[must_use]
    pub fn mono(mut self) -> Self {
        self.mono = true;
        self
    }

    /// The same line in `tone`.
    #[must_use]
    pub fn toned(
        mut self,
        tone: Tone,
    ) -> Self {
        self.tone = tone;
        self
    }
}
