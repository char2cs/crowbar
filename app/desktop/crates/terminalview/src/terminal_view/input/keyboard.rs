//! Keyboard handling of the view.

use gpui::{ClipboardItem, Context, KeyDownEvent, Window};

use crate::terminal_view::TerminalView;
use crate::terminal_view::input::key_conversion::key_input;
use crate::terminal_view::input::page_direction::PageDirection;
use crate::terminal_view::input::shortcut::{Shortcut, shortcut_for};
use crate::terminal_view::terminal_view_event::TerminalViewEvent;

const ZOOM_STEP: f32 = 2.0;

impl TerminalView {
    pub(crate) fn on_key_down(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let input = key_input(&event.keystroke);
        if let Some(shortcut) = shortcut_for(&input) {
            cx.stop_propagation();
            self.run_shortcut(shortcut, window, cx);
            return;
        }
        if input.key == "escape" && self.search.is_some() {
            cx.stop_propagation();
            self.close_search(window, cx);
            return;
        }
        let Some(bytes) = self.terminal.encode_key(&input) else {
            return;
        };
        cx.stop_propagation();
        self.send_bytes(&bytes, cx);
    }

    fn run_shortcut(
        &mut self,
        shortcut: Shortcut,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match shortcut {
            Shortcut::Copy => self.copy_selection(cx),
            Shortcut::Paste => self.paste_clipboard(cx),
            Shortcut::SelectAll => self.select_all(cx),
            Shortcut::Find => self.open_search(window, cx),
            Shortcut::ZoomIn => self.zoom(ZOOM_STEP, cx),
            Shortcut::ZoomOut => self.zoom(-ZOOM_STEP, cx),
            Shortcut::ZoomReset => self.reset_zoom(cx),
            Shortcut::ClearScrollback => self.clear_scrollback(cx),
            Shortcut::PageUp => self.scroll_page(PageDirection::Up, cx),
            Shortcut::PageDown => self.scroll_page(PageDirection::Down, cx),
        }
    }

    fn copy_selection(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(text) = self.terminal.selected_text() {
            cx.write_to_clipboard(ClipboardItem::new_string(text));
        }
    }

    fn paste_clipboard(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = cx.read_from_clipboard().and_then(|item| item.text()) else {
            return;
        };
        self.paste(&text, cx);
    }

    fn zoom(
        &mut self,
        delta: f32,
        cx: &mut Context<'_, Self>,
    ) {
        let zoomed = self.settings.zoomed(delta);
        self.apply_font_size(zoomed.font_size, cx);
    }

    fn reset_zoom(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let reset = self.settings.zoom_reset();
        self.apply_font_size(reset.font_size, cx);
    }

    fn apply_font_size(
        &mut self,
        size: f32,
        cx: &mut Context<'_, Self>,
    ) {
        self.settings.font_size = size;
        self.cache.clear();
        cx.emit(TerminalViewEvent::FontSizeChanged(size));
        cx.notify();
    }

    fn scroll_page(
        &mut self,
        direction: PageDirection,
        cx: &mut Context<'_, Self>,
    ) {
        match direction {
            PageDirection::Up => self.terminal.scroll_page_up(),
            PageDirection::Down => self.terminal.scroll_page_down(),
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ClipboardItem, TestAppContext};

    use crate::terminal_view::TerminalView;
    use crate::terminal_view::terminal_view_event::TerminalViewEvent;
    use crate::test_support::{feed, first_row, focus, open, record_events, typed};

    #[gpui::test]
    fn typing_goes_to_the_daemon_as_input(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        focus(&harness, cx);
        cx.simulate_keystrokes("l s enter");
        assert!(typed(&harness).ends_with("ls\r"));
    }

    #[gpui::test]
    fn control_keys_and_arrows_use_terminal_encodings(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        focus(&harness, cx);
        cx.simulate_keystrokes("ctrl-c up");
        assert!(typed(&harness).ends_with("\x03\x1b[A"));
    }

    #[gpui::test]
    fn paste_shortcut_sends_the_clipboard_bracketed_when_asked(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "\x1b[?2004h");
        focus(&harness, cx);
        cx.write_to_clipboard(ClipboardItem::new_string("a b".into()));
        cx.simulate_keystrokes("ctrl-shift-v");
        assert!(typed(&harness).ends_with("\x1b[200~a b\x1b[201~"));
    }

    #[gpui::test]
    fn zoom_reports_the_new_font_size_and_clear_drops_history(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        focus(&harness, cx);
        let events = record_events::<TerminalView, TerminalViewEvent>(cx, &harness.view);
        cx.simulate_keystrokes("ctrl-=");
        assert_eq!(
            events.borrow().as_slice(),
            [TerminalViewEvent::FontSizeChanged(16.0)]
        );
        feed(&harness, cx, &"x\r\n".repeat(200));
        harness.view.update(cx, TerminalView::clear_scrollback);
        let history = harness
            .view
            .read_with(cx, |view, _| view.terminal.snapshot().history_size);
        assert_eq!(history, 0);
        assert!(!first_row(&harness, cx).is_empty() || history == 0);
    }
}
