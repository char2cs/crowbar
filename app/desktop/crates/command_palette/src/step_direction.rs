#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum StepDirection {
    Up,
    Down,
}

impl StepDirection {
    pub(crate) fn from_key(key: &str) -> Option<Self> {
        match key {
            "down" => Some(Self::Down),
            "up" => Some(Self::Up),
            _ => None,
        }
    }

    pub(crate) fn apply(
        self,
        selected: usize,
        len: usize,
    ) -> usize {
        match self {
            Self::Down => (selected + 1).min(len.saturating_sub(1)),
            Self::Up => selected.saturating_sub(1),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stepping_stays_inside_the_list() {
        let cases = [
            (StepDirection::Down, 0, 3, 1),
            (StepDirection::Down, 2, 3, 2),
            (StepDirection::Up, 1, 3, 0),
            (StepDirection::Up, 0, 3, 0),
            (StepDirection::Down, 0, 0, 0),
        ];
        for (direction, selected, len, expected) in cases {
            assert_eq!(
                direction.apply(selected, len),
                expected,
                "{direction:?} {selected}/{len}"
            );
        }
    }

    #[test]
    fn only_the_arrow_keys_step() {
        let cases = [
            ("down", Some(StepDirection::Down)),
            ("up", Some(StepDirection::Up)),
            ("enter", None),
        ];
        for (key, expected) in cases {
            assert_eq!(StepDirection::from_key(key), expected, "{key}");
        }
    }
}
