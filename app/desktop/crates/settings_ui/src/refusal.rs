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
mod tests {
    use super::*;

    #[test]
    fn every_refusal_explains_itself() {
        for refusal in [Refusal::BareKey, Refusal::UnsupportedModifier] {
            assert!(!refusal.message().is_empty(), "{refusal:?}");
        }
    }
}
