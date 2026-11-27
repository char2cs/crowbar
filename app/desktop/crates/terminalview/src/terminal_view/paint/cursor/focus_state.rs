const FOCUS_IN: &str = "\x1b[I";
const FOCUS_OUT: &str = "\x1b[O";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum FocusState {
    Focused,
    Unfocused,
}

impl From<bool> for FocusState {
    fn from(focused: bool) -> Self {
        if focused {
            return Self::Focused;
        }
        Self::Unfocused
    }
}

impl FocusState {
    pub(crate) fn report(self) -> &'static str {
        match self {
            Self::Focused => FOCUS_IN,
            Self::Unfocused => FOCUS_OUT,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn focus_reports_use_the_xterm_sequences() {
        assert_eq!(FocusState::Focused.report(), "\x1b[I");
        assert_eq!(FocusState::Unfocused.report(), "\x1b[O");
        assert_eq!(FocusState::from(true), FocusState::Focused);
        assert_eq!(FocusState::from(false), FocusState::Unfocused);
    }
}
