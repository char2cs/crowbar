//! One terminal row together with what shaping it needs.

use terminal::Row;

use crate::terminal_view::paint::shaping::Shaping;

#[derive(Debug, Clone, Copy)]
pub(crate) struct RowInput<'a> {
    pub(crate) index: usize,
    pub(crate) row: &'a Row,
    pub(crate) shaping: &'a Shaping,
}

impl<'a> RowInput<'a> {
    pub(crate) fn new(
        index: usize,
        row: &'a Row,
        shaping: &'a Shaping,
    ) -> Self {
        Self {
            index,
            row,
            shaping,
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::px;

    use super::*;

    #[test]
    fn the_input_keeps_the_row_it_was_given() {
        let shaping = Shaping {
            font: gpui::font("monospace"),
            font_size: px(12.0),
            cell_width: px(8.0),
        };
        let row = Row::default();
        let input = RowInput::new(3, &row, &shaping);
        assert_eq!(input.index, 3);
        assert_eq!(input.row, &row);
    }
}
