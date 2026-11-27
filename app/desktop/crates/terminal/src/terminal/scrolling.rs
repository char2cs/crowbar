use alacritty_terminal::grid::Scroll;

use crate::terminal::Terminal;

impl Terminal {
    pub fn scroll_lines(
        &mut self,
        lines: i32,
    ) {
        self.term.scroll_display(Scroll::Delta(lines));
    }

    pub fn scroll_page_up(&mut self) {
        self.term.scroll_display(Scroll::PageUp);
    }

    pub fn scroll_page_down(&mut self) {
        self.term.scroll_display(Scroll::PageDown);
    }

    pub fn scroll_to_top(&mut self) {
        self.term.scroll_display(Scroll::Top);
    }

    pub fn scroll_to_bottom(&mut self) {
        self.term.scroll_display(Scroll::Bottom);
    }

    pub fn set_display_offset(
        &mut self,
        offset: usize,
    ) {
        let current = i64::try_from(self.term.grid().display_offset()).unwrap_or(0);
        let wanted = i64::try_from(offset).unwrap_or(0);
        self.scroll_lines(i32::try_from(wanted - current).unwrap_or(0));
    }

    pub fn clear_scrollback(&mut self) {
        self.term.grid_mut().clear_history();
        self.term.selection = None;
    }
}

#[cfg(test)]
mod tests {
    use crate::terminal::wire::DaemonFrame;
    use crate::test_support::empty_terminal;

    #[test]
    fn bulk_output_fills_the_scrollback_and_resize_reflows() {
        let mut terminal = empty_terminal(40, 6);
        let bulk = "row\r\n".repeat(500);
        terminal.apply(&DaemonFrame::Output(bulk.into_bytes()));
        assert!(terminal.snapshot().history_size > 400);
        terminal.resize(20, 4);
        assert_eq!(terminal.size(), (20, 4));
        terminal.scroll_to_top();
        assert!(terminal.snapshot().display_offset > 0);
        terminal.clear_scrollback();
        assert_eq!(terminal.snapshot().history_size, 0);
    }
}
