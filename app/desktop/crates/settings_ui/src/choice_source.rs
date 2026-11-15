//! Where a select gets its options.

use crate::choice::Choice;

/// Fixed options, or a named provider the backend answers at open (themes, fonts).
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum ChoiceSource {
    /// Options known to the schema.
    Fixed(Vec<Choice>),
    /// Options the backend lists under this name.
    Provided(String),
}

impl ChoiceSource {
    /// Fixed options from `(value, label)` pairs.
    pub fn fixed(pairs: &[(&str, &str)]) -> Self {
        Self::Fixed(
            pairs
                .iter()
                .map(|&(value, label)| Choice::new(value, label))
                .collect(),
        )
    }

    /// Options listed by the backend provider `name`.
    pub fn provided(name: impl Into<String>) -> Self {
        Self::Provided(name.into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fixed_pairs_become_choices_in_order() {
        let source = ChoiceSource::fixed(&[("left", "Left"), ("right", "Right")]);
        assert_eq!(
            source,
            ChoiceSource::Fixed(vec![
                Choice::new("left", "Left"),
                Choice::new("right", "Right")
            ])
        );
        assert_eq!(
            ChoiceSource::provided("themes"),
            ChoiceSource::Provided("themes".to_owned())
        );
    }
}
