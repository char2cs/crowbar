use crate::FindOption;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FindOptions {
    on: Vec<FindOption>,
}

impl FindOptions {
    #[must_use]
    pub fn of(on: &[FindOption]) -> Self {
        Self { on: on.to_vec() }
    }

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
