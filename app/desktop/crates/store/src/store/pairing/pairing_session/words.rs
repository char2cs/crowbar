pub(crate) mod already_paired;
pub(crate) mod failure;
mod refusal;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Words(String);

impl Words {
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
        assert_eq!(Words::new("Can’t reach".into()).into_text(), "Can’t reach");
    }
}
