use alacritty_terminal::grid::Dimensions;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct GridSize {
    pub(crate) columns: usize,
    pub(crate) lines: usize,
}

impl Dimensions for GridSize {
    fn total_lines(&self) -> usize {
        self.lines
    }

    fn screen_lines(&self) -> usize {
        self.lines
    }

    fn columns(&self) -> usize {
        self.columns
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dimensions_report_the_screen_without_history() {
        let size = GridSize {
            columns: 80,
            lines: 24,
        };
        assert_eq!(
            (size.columns(), size.screen_lines(), size.total_lines()),
            (80, 24, 24)
        );
    }
}
