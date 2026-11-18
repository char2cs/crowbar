//! Whether a text area accepts input and whether its content is flagged.

/// How a [`crate::TextArea`] presents itself.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TextAreaCondition {
    /// Editable.
    #[default]
    Normal,
    /// Editable, with the destructive border of `aria-invalid`.
    Invalid,
    /// Not focusable, not editable, drawn at 64% opacity.
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
