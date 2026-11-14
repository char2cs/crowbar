//! Why a captured key press is not accepted as a chord.

/// A reason the capture rule turns a key press down.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Refusal {
    /// A printable or navigation key without a modifier would shadow typing.
    BareKey,
    /// A modifier the shortcut grammar has no word for.
    UnsupportedModifier,
}

impl Refusal {
    /// The hint shown beside the capture button.
    pub fn message(self) -> &'static str {
        match self {
            Self::BareKey => "Hold Ctrl or Alt, or use a function key",
            Self::UnsupportedModifier => "That modifier cannot be used here",
        }
    }
}

#[cfg(test)]
#[path = "refusal_tests.rs"]
mod tests;
