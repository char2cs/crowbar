//! What a host calls on a terminal view.

use gpui::{Context, SharedString, Window};

use crate::settings::Settings;
use crate::settings::terminal_config;
use crate::terminal_view::TerminalView;

impl TerminalView {
    /// Moves keyboard focus to the terminal.
    pub fn focus(
        &self,
        window: &mut Window,
    ) {
        window.focus(&self.focus);
    }

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

    /// The directory relative paths in the output resolve against.
    pub fn set_working_directory(
        &mut self,
        directory: Option<String>,
    ) {
        self.working_directory = directory;
    }

    /// Types `command` and Enter once the shell has drawn its first output.
    pub fn run_after_attach(
        &mut self,
        command: impl Into<String>,
    ) {
        self.pending_command = Some(command.into());
    }

    /// Covers the terminal with a message (host unavailable, waiting); `None` removes it.
    pub fn set_notice(
        &mut self,
        notice: Option<SharedString>,
        cx: &mut Context<'_, Self>,
    ) {
        self.notice = notice;
        cx.notify();
    }

    /// Drops the 16px left inset a shell tab has; an agent pane's frame supplies its own.
    pub fn set_flush(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.inset = 0.0;
        cx.notify();
    }

    /// Pastes `text` as if typed, bracketed when the program asked for it.
    pub fn paste(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let bytes = self.terminal.encode_paste(text);
        self.send_bytes(&bytes, cx);
    }

    /// Selects everything, scrollback included.
    pub fn select_all(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.terminal.select_all();
        cx.notify();
    }

    /// The selected text.
    pub fn selected_text(&self) -> Option<String> {
        self.terminal.selected_text()
    }

    /// Drops the scrollback.
    pub fn clear_scrollback(
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
        harness.view.update(cx, |view, cx| view.paste("a\nb", cx));
        assert_eq!(typed(&harness), "a\rb");
        feed(&harness, cx, "all of it");
        harness.view.update(cx, TerminalView::select_all);
        let selected = harness.view.read_with(cx, |view, _| view.selected_text());
        assert!(selected.is_some_and(|text| text.starts_with("all of it")));
        let settings = Settings::default().zoomed(2.0);
        harness
            .view
            .update(cx, |view, cx| view.set_settings(settings, cx));
        assert!(harness.view.read_with(cx, |view, _| view.grid_size().0 > 0));
    }

    #[gpui::test]
    fn a_working_directory_and_flush_inset_can_be_set(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        harness.view.update(cx, |view, _| {
            view.set_working_directory(Some("/work".into()));
        });
        harness.view.update(cx, TerminalView::set_flush);
        let inset = harness.view.read_with(cx, |view, _| view.inset);
        assert_eq!(inset, 0.0);
    }
}
