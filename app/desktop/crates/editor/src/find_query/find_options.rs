use crate::FindOption;

/// The find toggles that are on; all are off at first, as in Monaco.
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

    /// These options with `option` flipped.
    #[must_use]
    pub fn toggled(
        &self,
        option: FindOption,
    ) -> Self {
        if self.has(option) {
            let on = self
                .on
                .iter()
                .copied()
                .filter(|kept| *kept != option)
                .collect();
            return Self { on };
        }
        let on = self.on.iter().copied().chain([option]).collect();
        Self { on }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toggling_an_option_turns_it_on_then_off_leaving_the_others() {
        let both = FindOptions::of(&[FindOption::Regex]).toggled(FindOption::MatchCase);
        assert!(both.has(FindOption::Regex) && both.has(FindOption::MatchCase));
        let back = both.toggled(FindOption::MatchCase);
        assert_eq!(back, FindOptions::of(&[FindOption::Regex]));
    }
}
