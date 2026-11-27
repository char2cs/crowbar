use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub enum Visibility {
    #[default]
    Open,
    Closed,
}

impl Visibility {
    #[must_use]
    pub fn flipped(self) -> Self {
        match self {
            Self::Open => Self::Closed,
            Self::Closed => Self::Open,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn flipping_twice_returns_the_start() {
        for start in [Visibility::Open, Visibility::Closed] {
            assert_ne!(start.flipped(), start, "flip changes the value");
            assert_eq!(start.flipped().flipped(), start, "flip is an involution");
        }
    }
}
