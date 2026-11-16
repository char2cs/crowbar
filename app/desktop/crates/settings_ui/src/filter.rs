//! How much of a page the search query lets through.

#[derive(Clone, Copy)]
pub(crate) enum Filter<'a> {
    Everything,
    Words(&'a [String]),
}

impl Filter<'_> {
    pub(crate) fn admits(
        self,
        texts: &[&str],
    ) -> bool {
        match self {
            Self::Everything => true,
            Self::Words(words) => matches(words, texts),
        }
    }
}

pub(crate) fn matches(
    words: &[String],
    texts: &[&str],
) -> bool {
    let haystack = texts.join("\n").to_lowercase();
    words.iter().all(|word| haystack.contains(word.as_str()))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn words(text: &str) -> Vec<String> {
        text.split_whitespace().map(str::to_owned).collect()
    }

    #[test]
    fn everything_admits_any_text() {
        assert!(Filter::Everything.admits(&["unrelated"]));
    }

    #[test]
    fn words_admit_only_text_holding_every_word_in_any_case() {
        let query = words("font size");
        let filter = Filter::Words(&query);
        assert!(filter.admits(&["UI Font", "Size of the text"]));
        assert!(!filter.admits(&["UI Font"]));
    }
}
