//! The terminal as a GPUI view: grid painting, keyboard, mouse, IME, find bar and scrollbar.

mod api;
mod event;
mod input;
mod paint;
mod search;
mod session;

pub use event::Event;

use std::time::Instant;

use async_channel::{Receiver, Sender};
use gpui::{
    App, AsyncApp, Context, Div, EventEmitter, ExternalPaths, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, ParentElement, Render, SharedString, Stateful,
    Styled, Subscription, Task, WeakEntity, Window, canvas, div,
};
use terminal::{
    ClickTracker, ClientMessage, CursorCell, DaemonFrame, Link, ScrollAccumulator, Terminal,
};
use ui::Theme;

use crate::daemon;
use crate::daemon::take_batch;
use crate::search_bar::Session;
use crate::settings::Settings;
use crate::settings::terminal_config;
use crate::terminal_view::input::pointer::auto_scroll::AutoScroll;
use crate::terminal_view::input::pointer::button_event::ButtonEvent;
use crate::terminal_view::input::pointer::drag_state::DragState;
use crate::terminal_view::paint::colors::Colors;
use crate::terminal_view::paint::cursor::blink::Blink;
use crate::terminal_view::paint::request::Request;
use crate::terminal_view::paint::shaping::line_cache::LineCache;
use crate::terminal_view::paint::view_layout::ViewLayout;

const NOTICE_BACKDROP_ALPHA: f32 = 0.8;

/// A terminal attached to a daemon PTY through a [`daemon::Link`].
pub struct TerminalView {
    pub(crate) terminal: Terminal,
    pub(crate) outbound: Sender<ClientMessage>,
    pub(crate) settings: Settings,
    pub(crate) focus: FocusHandle,
    pub(crate) focused: bool,
    pub(crate) layout: Option<ViewLayout>,
    pub(crate) reported_size: Option<(usize, usize)>,
    pub(crate) clicks: ClickTracker,
    pub(crate) epoch: Instant,
    pub(crate) wheel: ScrollAccumulator,
    pub(crate) drag: DragState,
    pub(crate) hovered: Option<Link>,
    pub(crate) pointer: Option<gpui::Point<gpui::Pixels>>,
    pub(crate) blink: Blink,
    pub(crate) search: Option<Session>,
    pub(crate) preedit: Option<String>,
    pub(crate) unread: Option<Receiver<DaemonFrame>>,
    pub(crate) pending_command: Option<String>,
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

impl EventEmitter<Event> for TerminalView {}

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
        link: daemon::Link,
        settings: Settings,
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
            epoch: cx.background_executor().now(),
            wheel: ScrollAccumulator::default(),
            drag: DragState::Idle,
            hovered: None,
            pointer: None,
            blink: Blink::default(),
            search: None,
            preedit: None,
            unread: Some(link.frames),
            pending_command: None,
            notice: None,
            cache: LineCache::default(),
            cursor_cell: None,
            auto_scroll: None,
            inset: ViewLayout::default_inset(),
            subscriptions: Vec::new(),
            tasks: Vec::new(),
        };
        view.start_blink_task(cx);
        view.watch_focus(window, cx);
        view.watch_theme(cx);
        view.push_theme(cx);
        view
    }

    pub(crate) fn start_frame_task(
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
        window.focus(&self.focus, cx);
        if !text.is_empty() {
            self.paste_text(&text, cx);
        }
    }
}

impl Render for TerminalView {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let colors = Colors::of(cx);
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
                let request = Request::new(bounds, frame);
                view.update(cx, |this, cx| this.paint_frame(request, window, cx));
            },
        )
        .size_full()
    }

    fn root(
        &self,
        colors: &Colors,
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
            .on_scroll(cx.listener(Self::on_scroll))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .on_drop(cx.listener(Self::on_drop))
            .child(Self::surface(cx));
        [MouseButton::Left, MouseButton::Middle, MouseButton::Right]
            .into_iter()
            .fold(root, |root, button| with_button(root, button, cx))
    }

    fn notice_element(
        &self,
        colors: &Colors,
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
        cx.listener(move |this, event, window, cx| {
            this.on_mouse_down(ButtonEvent { button, event }, window, cx);
        }),
    )
    .on_mouse_up(
        button,
        cx.listener(move |this, event, _, cx| this.on_mouse_up(ButtonEvent { button, event }, cx)),
    )
    .on_mouse_up_out(
        button,
        cx.listener(move |this, event, _, cx| this.on_mouse_up(ButtonEvent { button, event }, cx)),
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
    use gpui::{AppContext as _, TestAppContext, point, px};
    use terminal::{ClientMessage, DaemonFrame};

    use super::TerminalView;
    use crate::daemon::Link;
    use crate::settings::Settings;
    use crate::test_support::{draw, feed, first_row, open, sent};

    #[gpui::test]
    fn a_screen_sent_before_the_first_layout_keeps_its_cursor_on_its_prompt(
        cx: &mut TestAppContext
    ) {
        let (harness, cx) = open(cx);
        let lines = harness.view.read_with(cx, |view, _| view.terminal.size().1);
        let (link, peer) = Link::open();
        let late = cx.update(|window, cx| {
            cx.new(|cx| TerminalView::new(link, Settings::default(), window, cx))
        });
        let screen = format!("again\r\n$ {}\u{1b}[2;3H", "\r\n".repeat(lines - 2));
        peer.deliver_blocking(DaemonFrame::Snapshot(screen.into_bytes()));
        cx.run_until_parked();
        let size = cx.update(|window, _| window.viewport_size());
        let origin = point(px(0.0), px(0.0));
        cx.draw(origin, size, |_, _| {
            gpui::IntoElement::into_any_element(late.clone())
        });
        cx.run_until_parked();
        let row = late.read_with(cx, |view, _| view.terminal.snapshot().cursor.map(|c| c.row));
        assert_eq!(row, Some(1), "the cursor stays after the prompt");
    }

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
