//! A flag of a daemon record, as an enum so no struct or signature carries a bare bool.

use serde::Deserialize;

/// A flag of a daemon record; a missing or `null` flag reads as off.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Deserialize)]
#[serde(from = "Option<bool>")]
pub enum Switch {
    /// The flag is set.
    On,
    /// The flag is unset.
    #[default]
    Off,
}

impl From<Option<bool>> for Switch {
    fn from(on: Option<bool>) -> Self {
        match on {
            Some(true) => Self::On,
            _ => Self::Off,
        }
    }
}

impl Switch {
    /// Whether the flag is set.
    pub fn is_on(self) -> bool {
        self == Self::On
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_bool_converts_both_ways_and_parses_from_json() {
        assert_eq!(Switch::from(Some(true)), Switch::On);
        assert!(!Switch::from(Some(false)).is_on());
        assert_eq!(serde_json::from_str::<Switch>("true").unwrap(), Switch::On);
        assert_eq!(Switch::default(), Switch::Off);
        assert_eq!(Switch::from(None), Switch::Off);
    }
}
