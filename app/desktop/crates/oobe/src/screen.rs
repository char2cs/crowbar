#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub(crate) enum Screen {
    Welcome,
    SystemCheck,
    AddSpace,
    AddHost,
}

impl Screen {
    #[must_use]
    pub(crate) fn next(self) -> Self {
        match self {
            Self::Welcome => Self::SystemCheck,
            Self::SystemCheck | Self::AddSpace => Self::AddSpace,
            Self::AddHost => Self::AddHost,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn continuing_walks_welcome_check_space_and_stops() {
        const CASES: &[(Screen, Screen)] = &[
            (Screen::Welcome, Screen::SystemCheck),
            (Screen::SystemCheck, Screen::AddSpace),
            (Screen::AddSpace, Screen::AddSpace),
            (Screen::AddHost, Screen::AddHost),
        ];
        for &(from, to) in CASES {
            assert_eq!(from.next(), to, "{from:?}");
        }
    }
}
