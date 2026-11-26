//! The daemon side of the view: frames in, input, size and theme out, focus and blink.

use gpui::{AsyncApp, ClipboardItem, Context, WeakEntity, Window};
use terminal::{ClientMessage, DaemonFrame, sanitize_title};

use crate::settings::{hex_text, is_dark, terminal_palette};
use crate::terminal_view::TerminalView;
use crate::terminal_view::event::Event;
use crate::terminal_view::paint::BLINK_INTERVAL;
use crate::terminal_view::paint::FocusState;

impl TerminalView {
    pub(crate) fn apply_frames(
        &mut self,
        frames: &[DaemonFrame],
        cx: &mut Context<'_, Self>,
    ) {
        for event in self.terminal.apply_batch(frames) {
            Self::handle_event(event, cx);
        }
        if frames
            .iter()
            .any(|frame| matches!(frame, DaemonFrame::Snapshot(_)))
        {
            self.terminal.scroll_to_bottom();
        }
        cx.notify();
    }

    fn handle_event(
        event: terminal::Event,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            terminal::Event::Title(title) => {
                cx.emit(Event::TitleChanged(sanitize_title(&title)));
            },
            terminal::Event::ResetTitle => cx.emit(Event::TitleChanged(String::new())),
            terminal::Event::Bell => cx.emit(Event::Bell),
            terminal::Event::ClipboardStore(text) => {
                cx.emit(Event::ClipboardWritten(text.len()));
                cx.write_to_clipboard(ClipboardItem::new_string(text));
            },
            terminal::Event::Exit(code) => cx.emit(Event::Exited(code)),
        }
    }

    pub(crate) fn send(
        &self,
        message: ClientMessage,
    ) {
        if let Err(error) = self.outbound.try_send(message) {
            tracing::debug!(%error, "terminal message dropped: the connection is gone");
        }
    }

    pub(crate) fn send_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.blink.restart();
        if self.terminal.snapshot().display_offset > 0 {
            self.terminal.scroll_to_bottom();
        }
        self.send(ClientMessage::Input(text.to_owned()));
        cx.notify();
    }

    pub(crate) fn send_bytes(
        &mut self,
        bytes: &[u8],
        cx: &mut Context<'_, Self>,
    ) {
        let text = String::from_utf8_lossy(bytes).into_owned();
        self.send_text(&text, cx);
    }

    pub(crate) fn push_theme(
        &self,
        cx: &gpui::App,
    ) {
        let palette = terminal_palette(cx);
        self.send(ClientMessage::Theme {
            background: hex_text(palette.background),
            foreground: hex_text(palette.foreground),
            dark: is_dark(cx),
        });
    }

    pub(crate) fn start_blink_task(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let task = cx.spawn(async move |view, cx| {
            blink_loop(view, cx).await;
        });
        self.tasks.push(task);
    }

    fn blink_tick(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.focused || !self.terminal.cursor_blinks() {
            return;
        }
        self.blink.tick();
        cx.notify();
    }

    pub(crate) fn watch_focus(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let focus = self.focus.clone();
        let entered = cx.on_focus_in(&focus, window, |view, _, cx| {
            view.focus_changed(FocusState::Focused, cx);
        });
        let left = cx.on_focus_out(&focus, window, |view, _, _, cx| {
            view.focus_changed(FocusState::Unfocused, cx);
        });
        self.subscriptions.extend([entered, left]);
    }

    fn focus_changed(
        &mut self,
        state: FocusState,
        cx: &mut Context<'_, Self>,
    ) {
        self.focused = state == FocusState::Focused;
        self.blink.restart();
        if self.terminal.reports_focus() {
            self.send(ClientMessage::Input(state.report().to_owned()));
        }
        cx.notify();
    }
}

async fn blink_loop(
    view: WeakEntity<TerminalView>,
    cx: &mut AsyncApp,
) -> Option<()> {
    loop {
        cx.background_executor().timer(BLINK_INTERVAL).await;
        view.update(cx, TerminalView::blink_tick).ok()?;
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::terminal_view::TerminalView;
    use crate::terminal_view::event::Event;
    use crate::terminal_view::paint::BLINK_INTERVAL;
    use crate::test_support::{deliver, feed, focus, open, record_events, sent, typed};

    #[gpui::test]
    fn title_bell_and_exit_become_view_events(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        let events = record_events::<TerminalView, Event>(cx, &harness.view);
        feed(&harness, cx, "\x1b]0;build\x07\x07");
        deliver(&harness, terminal::DaemonFrame::Exit(2));
        cx.run_until_parked();
        let expected = [
            Event::TitleChanged("build".into()),
            Event::Bell,
            Event::Exited(2),
        ];
        assert_eq!(events.borrow().as_slice(), expected);
    }

    #[gpui::test]
    fn an_osc_52_store_writes_the_clipboard_and_tells_the_host(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        let events = record_events::<TerminalView, Event>(cx, &harness.view);
        feed(&harness, cx, "\x1b]52;c;aGk=\x07");
        assert_eq!(events.borrow().as_slice(), [Event::ClipboardWritten(2)]);
    }

    #[gpui::test]
    fn focus_changes_are_reported_only_when_the_program_asked(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        sent(&harness);
        focus(&harness, cx);
        cx.run_until_parked();
        assert_eq!(typed(&harness), "");
        feed(&harness, cx, "\x1b[?1004h");
        cx.update(gpui::Window::blur);
        cx.run_until_parked();
        focus(&harness, cx);
        cx.run_until_parked();
        assert_eq!(typed(&harness), "\x1b[O\x1b[I");
    }

    #[gpui::test]
    fn the_cursor_blinks_while_focused_and_restarts_on_typing(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "\x1b[1 q");
        focus(&harness, cx);
        cx.run_until_parked();
        cx.executor().advance_clock(BLINK_INTERVAL);
        cx.run_until_parked();
        assert!(!harness.view.read_with(cx, |view, _| view.blink.is_shown()));
        harness.view.update(cx, |view, cx| view.send_text("x", cx));
        assert!(harness.view.read_with(cx, |view, _| view.blink.is_shown()));
    }
}
