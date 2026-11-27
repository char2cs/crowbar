use crate::FindOption;

/// The find toggles that are on; all are off at first.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FindOptions {
    on: Vec<FindOption>,
}

impl FindOptions {
    /// Options with exactly `on` turned on.
    #[must_use]
    pub fn of(on: &[FindOption]) -> Self {
        Self { on: on.to_vec() }
    }

    /// Whether `option` is on.
    #[must_use]
    pub fn has(
        &self,
        option: FindOption,
    ) -> bool {
        self.on.contains(&option)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_options_asked_for_are_on() {
        let options = FindOptions::of(&[FindOption::WholeWord]);
        assert!(options.has(FindOption::WholeWord));
        assert!(!options.has(FindOption::MatchCase));
    }
}
