/// Which first-run screen is showing.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Screen {
    /// The brand, its tagline and the way in.
    Welcome,
    /// Git and the forge CLIs, each with its own answer.
    SystemCheck,
    /// The invitation to choose the first space's folder.
    AddSpace,
    /// The brand and the invitation to connect to a host; for a computer with no daemon of its own.
    AddHost,
}

impl Screen {
    /// The screen the "Continue" of this one leads to; the last screens stay.
    #[must_use]
    pub fn next(self) -> Self {
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
