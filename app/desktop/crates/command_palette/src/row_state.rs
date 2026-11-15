use gpui::Hsla;
use ui::Palette;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowState {
    Selected,
    Idle,
}

impl RowState {
    pub(crate) fn at(
        index: usize,
        selected: usize,
    ) -> Self {
        [Self::Idle, Self::Selected][usize::from(index == selected)]
    }

    pub(crate) fn fill(
        self,
        palette: Palette,
    ) -> Hsla {
        match self {
            Self::Selected => palette.accent(),
            Self::Idle => gpui::transparent_black(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_selected_index_is_highlighted() {
        const CASES: &[(usize, usize, RowState)] =
            &[(0, 0, RowState::Selected), (1, 0, RowState::Idle)];
        for (index, selected, expected) in CASES {
            assert_eq!(RowState::at(*index, *selected), *expected);
        }
    }
}
