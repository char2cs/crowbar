#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TextAreaCondition {
    #[default]
    Normal,
    Invalid,
    Disabled,
}

impl TextAreaCondition {
    pub(crate) fn accepts_input(self) -> bool {
        self != Self::Disabled
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_disabled_area_refuses_input() {
        const CASES: &[(TextAreaCondition, bool)] = &[
            (TextAreaCondition::Normal, true),
            (TextAreaCondition::Invalid, true),
            (TextAreaCondition::Disabled, false),
        ];
        for &(condition, accepts) in CASES {
            assert_eq!(condition.accepts_input(), accepts, "{condition:?}");
        }
    }
}
