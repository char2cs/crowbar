//! The terminal as a GPUI view: grid painting, keyboard, mouse, IME, find bar and scrollbar.

use std::time::Instant;

use async_channel::{Receiver, Sender};
use gpui::{
    App, AsyncApp, Context, Div, EventEmitter, ExternalPaths, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, ParentElement, Render, SharedString, Stateful,
    Styled, Subscription, Task, WeakEntity, Window, canvas, div,
};
use terminal::{
    ClickTracker, ClientMessage, CursorCell, DaemonFrame, Link, ScrollAccumulator, Terminal,
    TerminalConfig,
};
use ui::Theme;

use crate::auto_scroll::AutoScroll;
use crate::cursor_blink::CursorBlink;
use crate::drag_state::DragState;
use crate::frame_batch::take_batch;
use crate::line_cache::LineCache;
use crate::paint_colors::PaintColors;
use crate::search_session::SearchSession;
use crate::terminal_link::TerminalLink;
use crate::terminal_settings::TerminalSettings;
use crate::terminal_view_event::TerminalViewEvent;
use crate::theme_palette::terminal_palette;
use crate::view_layout::ViewLayout;

const INITIAL_COLUMNS: usize = 80;
const INITIAL_LINES: usize = 24;
const NOTICE_BACKDROP_ALPHA: f32 = 0.8;

/// A terminal attached to a daemon PTY through a [`TerminalLink`].
pub struct TerminalView {
    pub(crate) terminal: Terminal,
    pub(crate) outbound: Sender<ClientMessage>,
    pub(crate) settings: TerminalSettings,
    pub(crate) focus: FocusHandle,
    pub(crate) focused: bool,
    pub(crate) layout: Option<ViewLayout>,
    pub(crate) reported_size: Option<(usize, usize)>,
    pub(crate) clicks: ClickTracker,
    pub(crate) clock: Instant,
    pub(crate) wheel: ScrollAccumulator,
    pub(crate) drag: DragState,
    pub(crate) hovered: Option<Link>,
    pub(crate) blink: CursorBlink,
    pub(crate) search: Option<SearchSession>,
    pub(crate) preedit: Option<String>,
    pub(crate) pending_command: Option<String>,
    pub(crate) working_directory: Option<String>,
    pub(crate) notice: Option<SharedString>,
    pub(crate) cache: LineCache,
    pub(crate) cursor_cell: Option<CursorCell>,
    pub(crate) auto_scroll: Option<AutoScroll>,
    pub(crate) inset: f32,
    pub(crate) subscriptions: Vec<Subscription>,
    pub(crate) tasks: Vec<Task<()>>,
}

impl std::fmt::Debug for TerminalView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("TerminalView")
            .field("terminal", &self.terminal)
            .field("focused", &self.focused)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<TerminalViewEvent> for TerminalView {}

impl Focusable for TerminalView {
    fn focus_handle(
        &self,
        _: &App,
    ) -> FocusHandle {
        self.focus.clone()
    }
}

impl TerminalView {
    /// A terminal reading frames from `link` and writing input, size and theme to it.
    pub fn new(
        link: TerminalLink,
        settings: TerminalSettings,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let config = terminal_config(&settings, cx);
        let focus = cx.focus_handle();
        let mut view = Self {
            terminal: Terminal::new(config),
            outbound: link.outbound,
            settings,
            focus,
            focused: false,
            layout: None,
            reported_size: None,
            clicks: ClickTracker::default(),
            clock: Instant::now(),
            wheel: ScrollAccumulator::default(),
            drag: DragState::Idle,
            hovered: None,
            blink: CursorBlink::default(),
            search: None,
            preedit: None,
            pending_command: None,
            working_directory: None,
            notice: None,
            cache: LineCache::default(),
            cursor_cell: None,
            auto_scroll: None,
            inset: ViewLayout::default_inset(),
            subscriptions: Vec::new(),
            tasks: Vec::new(),
        };
        view.start_frame_task(link.frames, cx);
        view.start_blink_task(cx);
        view.watch_focus(window, cx);
        view.watch_theme(cx);
        view.push_theme(cx);
        view
    }

    fn start_frame_task(
        &mut self,
        frames: Receiver<DaemonFrame>,
        cx: &mut Context<'_, Self>,
    ) {
        let task = cx.spawn(async move |view, cx| {
            feed_frames(frames, view, cx).await;
        });
        self.tasks.push(task);
    }

    fn watch_theme(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let subscription = cx.observe_global::<Theme>(|view, cx| {
            let config = terminal_config(&view.settings, cx);
            view.terminal.reconfigure(config);
            view.cache.clear();
            view.push_theme(cx);
            cx.notify();
        });
        self.subscriptions.push(subscription);
    }

    fn on_drop(
        &mut self,
        paths: &ExternalPaths,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let text = terminal::format_dropped_paths(paths.paths());
        window.focus(&self.focus);
        if !text.is_empty() {
            self.send_text(&text, cx);
        }
    }
}

pub(crate) fn terminal_config(
    settings: &TerminalSettings,
    cx: &App,
) -> TerminalConfig {
    TerminalConfig {
        columns: INITIAL_COLUMNS,
        lines: INITIAL_LINES,
        scrollback: settings.scrollback,
        cursor_shape: settings.cursor_shape,
        cursor_blink: settings.cursor_blink,
        palette: terminal_palette(cx),
    }
}

impl Render for TerminalView {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let colors = PaintColors::of(cx);
        let root = self.root(&colors, cx);
        root.children(self.search.as_ref().map(|session| session.bar.clone()))
            .children(self.notice_element(&colors))
    }
}

impl TerminalView {
    fn surface(cx: &mut Context<'_, Self>) -> impl IntoElement + use<> {
        let view = cx.entity();
        let laid_out = view.clone();
        canvas(
            move |bounds, window, cx| {
                laid_out.update(cx, |this, cx| this.lay_out(bounds, window, cx))
            },
            move |bounds, frame, window, cx| {
                view.update(cx, |this, cx| this.paint_frame(bounds, frame, window, cx));
            },
        )
        .size_full()
    }

    fn root(
        &self,
        colors: &PaintColors,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        let root = div()
            .id("terminal")
            .key_context("Terminal")
            .relative()
            .size_full()
            .overflow_hidden()
            .bg(colors.pane)
            .track_focus(&self.focus)
            .cursor(self.pointer_style())
            .on_key_down(cx.listener(Self::on_key_down))
            .on_scroll_wheel(cx.listener(Self::on_scroll_wheel))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .on_drop(cx.listener(Self::on_drop))
            .child(Self::surface(cx));
        [MouseButton::Left, MouseButton::Middle, MouseButton::Right]
            .into_iter()
            .fold(root, |root, button| with_button(root, button, cx))
    }

    fn notice_element(
        &self,
        colors: &PaintColors,
    ) -> Option<impl IntoElement + use<>> {
        let message = self.notice.clone()?;
        Some(
            div()
                .absolute()
                .inset_0()
                .flex()
                .items_center()
                .justify_center()
                .bg(colors.pane.opacity(NOTICE_BACKDROP_ALPHA))
                .text_color(colors.notice_text)
                .child(message),
        )
    }
}

fn with_button(
    root: Stateful<Div>,
    button: MouseButton,
    cx: &mut Context<'_, TerminalView>,
) -> Stateful<Div> {
    root.on_mouse_down(
        button,
        cx.listener(move |this, event, window, cx| this.on_mouse_down(button, event, window, cx)),
    )
    .on_mouse_up(
        button,
        cx.listener(move |this, event, window, cx| this.on_mouse_up(button, event, window, cx)),
    )
    .on_mouse_up_out(
        button,
        cx.listener(move |this, event, window, cx| this.on_mouse_up(button, event, window, cx)),
    )
}

async fn feed_frames(
    frames: Receiver<DaemonFrame>,
    view: WeakEntity<TerminalView>,
    cx: &mut AsyncApp,
) -> Option<()> {
    loop {
        let first = frames.recv().await.ok()?;
        let batch = take_batch(first, &frames);
        view.update(cx, |view, cx| view.apply_frames(&batch, cx))
            .ok()?;
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use terminal::ClientMessage;

    use crate::test_support::{draw, feed, first_row, open, sent};

    #[gpui::test]
    fn frames_from_the_daemon_reach_the_screen(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "hello");
        assert_eq!(first_row(&harness, cx), "hello");
    }

    #[gpui::test]
    fn layout_reports_the_grid_size_and_the_theme(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        draw(cx);
        let messages = sent(&harness);
        let resized = messages.iter().any(|message| matches!(message, ClientMessage::Resize { columns, rows } if *columns > 0 && *rows > 0));
        let themed = messages
            .iter()
            .any(|message| matches!(message, ClientMessage::Theme { .. }));
        assert!(resized && themed, "{messages:?}");
    }

    #[gpui::test]
    fn a_notice_covers_the_terminal_until_cleared(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        harness.view.update(cx, |view, cx| {
            view.set_notice(Some("host offline".into()), cx);
        });
        assert!(harness.view.read_with(cx, |view, _| view.notice.is_some()));
        harness
            .view
            .update(cx, |view, cx| view.set_notice(None, cx));
        assert!(harness.view.read_with(cx, |view, _| view.notice.is_none()));
    }
}
