//! Mouse presses on the view: reporting to the program, starting a selection, pasting.

pub(crate) mod auto_scroll;
pub(crate) mod button_event;
mod drag;
pub(crate) mod drag_state;
mod release;
mod report;
mod wheel;

use gpui::{Context, CursorStyle, MouseButton, MouseDownEvent, Pixels, Point, Window};
use terminal::{CellHit, MouseAction, MouseEvent};
use terminal::{ClientMessage, MouseTracking};

use crate::terminal_view::TerminalView;
use crate::terminal_view::input::key_conversion::link_modifier_held;
use crate::terminal_view::input::pointer::button_event::ButtonEvent;
use crate::terminal_view::input::pointer::drag_state::DragState;
use crate::terminal_view::input::pointer::report::Report;
use crate::terminal_view::input::pointer::report::{pointer_report, reported_button};

impl TerminalView {
    pub(crate) fn pointer_style(&self) -> CursorStyle {
        self.hovered
            .as_ref()
            .map_or(CursorStyle::IBeam, |_| CursorStyle::PointingHand)
    }

    pub(crate) fn hit_at(
        &self,
        position: Point<Pixels>,
    ) -> Option<CellHit> {
        let layout = self.layout?;
        let (x, y) = layout.local(position);
        Some(layout.metrics.hit(x, y))
    }

    pub(crate) fn program_tracks_mouse(
        &self,
        modifiers: gpui::Modifiers,
    ) -> bool {
        self.terminal.mouse_tracking() != MouseTracking::Off && !modifiers.shift
    }

    pub(crate) fn on_mouse_down(
        &mut self,
        press: ButtonEvent<'_, MouseDownEvent>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let ButtonEvent { button, event } = press;
        window.focus(&self.focus);
        let Some(layout) = self.layout else {
            return;
        };
        if link_modifier_held(event.modifiers)
            && button == MouseButton::Left
            && self.open_link(event.position, cx)
        {
            return;
        }
        if button == MouseButton::Left && layout.track.contains(&event.position) {
            self.begin_thumb_drag(event.position, cx);
            return;
        }
        let Some(hit) = self.hit_at(event.position) else {
            return;
        };
        if self.program_tracks_mouse(event.modifiers) {
            let report = reported_button(button)
                .map(|reported| pointer_report(reported, hit.point, event.modifiers));
            self.report_press(report, cx);
            return;
        }
        self.begin_selection_or_paste(button, hit, cx);
    }

    pub(crate) fn report_press(
        &mut self,
        report: Option<Report>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(report) = report else {
            return;
        };
        self.drag = DragState::ReportingToProgram(report.button);
        self.report(MouseAction::Press, report, cx);
    }

    pub(crate) fn report(
        &mut self,
        action: MouseAction,
        report: Report,
        cx: &mut Context<'_, Self>,
    ) {
        let event = MouseEvent {
            button: report.button,
            action,
            point: report.point,
            modifiers: report.modifiers,
        };
        if let Some(bytes) = self.terminal.encode_mouse(&event) {
            self.send(ClientMessage::Input(
                String::from_utf8_lossy(&bytes).into_owned(),
            ));
            cx.notify();
        }
    }

    pub(crate) fn begin_selection_or_paste(
        &mut self,
        button: MouseButton,
        hit: CellHit,
        cx: &mut Context<'_, Self>,
    ) {
        match button {
            MouseButton::Left => {
                let elapsed = cx.background_executor().now().duration_since(self.epoch);
                let kind = self.clicks.press(hit.point, elapsed);
                self.terminal.begin_selection(hit.point, hit.side, kind);
                self.drag = DragState::Selecting;
                cx.notify();
            },
            #[cfg(any(target_os = "linux", target_os = "freebsd"))]
            MouseButton::Middle => self.paste_primary(cx),
            _ => {},
        }
    }

    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    pub(crate) fn paste_primary(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(text) = cx.read_from_primary().and_then(|item| item.text()) {
            self.paste(&text, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::{Modifiers, TestAppContext};

    use crate::terminal_view::TerminalView;
    use crate::terminal_view::event::Event;
    use crate::test_support::{cell_position, feed, open, press, record_events};

    const MULTI_CLICK_WINDOW: Duration = Duration::from_millis(500);

    #[gpui::test]
    fn a_modifier_click_on_a_url_asks_the_host_to_open_it(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "see https://x.dev/a now");
        let events = record_events::<TerminalView, Event>(cx, &harness.view);
        let position = cell_position(&harness, cx, 8);
        let modifiers = Modifiers {
            control: true,
            platform: true,
            ..Modifiers::default()
        };
        cx.simulate_event(press(position, modifiers));
        let opened = events
            .borrow()
            .iter()
            .any(|event| matches!(event, Event::OpenUrl { url, .. } if url == "https://x.dev/a"));
        assert!(opened, "{:?}", events.borrow());
    }

    #[gpui::test]
    fn two_presses_on_one_cell_select_a_word_only_while_the_window_is_open(
        cx: &mut TestAppContext
    ) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "hello world");
        let position = cell_position(&harness, cx, 2);
        let cases = [
            ("inside the window", MULTI_CLICK_WINDOW / 2, true),
            ("past it", MULTI_CLICK_WINDOW * 2, false),
        ];
        for (name, gap, selects_word) in cases {
            cx.simulate_event(press(position, Modifiers::default()));
            cx.executor().advance_clock(gap);
            cx.simulate_event(press(position, Modifiers::default()));
            let selected = harness.view.read_with(cx, |view, _| view.selected_text());
            assert_eq!(
                selected.is_some_and(|text| text.starts_with("hello")),
                selects_word,
                "{name}"
            );
            cx.executor().advance_clock(MULTI_CLICK_WINDOW * 2);
        }
    }
}
