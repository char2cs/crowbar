#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Refusal {
    BareKey,
    UnsupportedModifier,
}

impl Refusal {
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
