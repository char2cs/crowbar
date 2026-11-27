use gpui::{Pixels, ShapedLine};
use terminal::Row;

#[derive(Clone, Copy)]
pub(crate) struct RowLines<'a> {
    pub(crate) top: Pixels,
    pub(crate) row: &'a Row,
    pub(crate) lines: &'a [ShapedLine],
}

impl<'a> RowLines<'a> {
    pub(crate) fn new(
        top: Pixels,
        row: &'a Row,
        lines: &'a [ShapedLine],
    ) -> Self {
        Self { top, row, lines }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_painted_row_keeps_its_top_and_lines() {
        let row = Row::default();
        let painted = RowLines::new(gpui::px(12.0), &row, &[]);
        assert_eq!(painted.top, gpui::px(12.0));
        assert!(painted.lines.is_empty());
    }
}
