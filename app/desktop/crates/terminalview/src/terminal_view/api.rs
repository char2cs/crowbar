//! What a host calls on a terminal view.

use gpui::Context;

use crate::settings::Settings;
use crate::settings::terminal_config;
use crate::terminal_view::TerminalView;

impl TerminalView {
    /// Applies new settings: font, spacing, scrollback, cursor.
    pub fn set_settings(
        &mut self,
        settings: Settings,
        cx: &mut Context<'_, Self>,
    ) {
        let config = terminal_config(&settings, cx);
        self.terminal.reconfigure(config);
        self.settings = settings;
        self.cache.clear();
        cx.notify();
    }

    pub(crate) fn paste_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let bytes = self.terminal.encode_paste(text);
        self.send_bytes(&bytes, cx);
    }

    pub(crate) fn select_all(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.terminal.select_all();
        cx.notify();
    }

    pub(crate) fn clear_scrollback(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.terminal.clear_scrollback();
        cx.notify();
    }

    /// Columns and lines of the grid.
    pub fn grid_size(&self) -> (usize, usize) {
        self.terminal.size()
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::settings::Settings;
    use crate::terminal_view::TerminalView;
    use crate::test_support::{feed, open, typed};

    #[gpui::test]
    fn paste_select_all_and_settings_work_through_the_public_api(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        harness
            .view
            .update(cx, |view, cx| view.paste_text("a\nb", cx));
        assert_eq!(typed(&harness), "a\rb");
        feed(&harness, cx, "all of it");
        harness.view.update(cx, TerminalView::select_all);
        let selected = harness.view.read_with(cx, |view, _| view.terminal.selected_text());
        assert!(selected.is_some_and(|text| text.starts_with("all of it")));
        let settings = Settings::default().zoomed(2.0);
        harness
            .view
            .update(cx, |view, cx| view.set_settings(settings, cx));
        assert!(harness.view.read_with(cx, |view, _| view.grid_size().0 > 0));
    }
}
