use gpui::FontWeight;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Emphasis {
    Plain,
    Matched,
}

impl Emphasis {
    pub(crate) fn at(
        index: usize,
        positions: &[usize],
    ) -> Self {
        if positions.contains(&index) {
            return Self::Matched;
        }
        Self::Plain
    }

    pub(crate) fn weight(self) -> FontWeight {
        match self {
            Self::Plain => FontWeight::NORMAL,
            Self::Matched => FontWeight::BOLD,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matched_positions_are_bold() {
        const CASES: &[(usize, Emphasis)] = &[(0, Emphasis::Matched), (1, Emphasis::Plain)];
        for (index, expected) in CASES {
            assert_eq!(Emphasis::at(*index, &[0]), *expected);
        }
    }
}
