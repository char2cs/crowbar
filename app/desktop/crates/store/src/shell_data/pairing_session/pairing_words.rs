//! What a failed step of pairing says to the user, in plain words.

pub(crate) mod already_paired;
pub(crate) mod failure;
mod refusal;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PairingWords(String);

impl PairingWords {
    pub(crate) fn new(text: String) -> Self {
        Self(text)
    }

    pub(crate) fn into_text(self) -> String {
        self.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_words_are_handed_back_as_they_were_given() {
        assert_eq!(
            PairingWords::new("Can’t reach".into()).into_text(),
            "Can’t reach"
        );
    }
}
