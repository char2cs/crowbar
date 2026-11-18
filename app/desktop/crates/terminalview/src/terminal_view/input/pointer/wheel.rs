use std::cmp::Ordering;

use gpui::{Context, Pixels, Point, ScrollDelta, ScrollWheelEvent, Window};
use terminal::{MouseAction, ScrollbarThumb, ViewPoint, WheelRoute, route_wheel};

use crate::terminal_view::TerminalView;
use crate::terminal_view::input::key_conversion::terminal_modifiers;
use crate::terminal_view::input::pointer::drag_state::DragState;
use crate::terminal_view::input::pointer::report::Report;
use crate::terminal_view::input::pointer::report::pointer_report;

const WHEEL_LINES_PER_NOTCH: f32 = 3.0;
const HALF: f32 = 2.0;

impl TerminalView {
    pub(crate) fn scrollbar_thumb(&self) -> Option<ScrollbarThumb> {
        let layout = self.layout?;
        let snapshot = self.terminal.snapshot();
        ScrollbarThumb::new(
            f32::from(layout.track.size.height),
            layout.metrics.lines,
            snapshot.history_size,
            snapshot.display_offset,
        )
    }

    pub(crate) fn begin_thumb_drag(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(layout), Some(thumb)) = (self.layout, self.scrollbar_thumb()) else {
            return;
        };
        let pointer = f32::from(position.y - layout.track.origin.y);
        let grab = grab_offset(pointer, thumb);
        self.drag = DragState::ScrollbarThumb { grab };
        self.drag_thumb(position, grab, cx);
    }

    pub(crate) fn drag_thumb(
        &mut self,
        position: Point<Pixels>,
        grab: f32,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(layout), Some(thumb)) = (self.layout, self.scrollbar_thumb()) else {
            return;
        };
        let track = f32::from(layout.track.size.height);
        let top = f32::from(position.y - layout.track.origin.y) - grab;
        let history = self.terminal.snapshot().history_size;
        self.terminal.set_display_offset(ScrollbarThumb::offset_at(
            track,
            thumb.length,
            history,
            top,
        ));
        cx.notify();
    }

    pub(crate) fn on_scroll_wheel(
        &mut self,
        event: &ScrollWheelEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(layout) = self.layout else {
            return;
        };
        let lines = match event.delta {
            ScrollDelta::Pixels(delta) => f32::from(delta.y) / layout.metrics.line_height,
            ScrollDelta::Lines(delta) => delta.y * WHEEL_LINES_PER_NOTCH,
        };
        let whole = self.wheel.add(lines);
        if whole == 0 {
            return;
        }
        let modifiers = terminal_modifiers(event.modifiers);
        let route = route_wheel(
            self.terminal.mouse_tracking(),
            self.terminal.screen_buffer(),
            modifiers,
        );
        let point = self
            .hit_at(event.position)
            .map_or_else(ViewPoint::default, |hit| hit.point);
        match route {
            WheelRoute::Scrollback => self.terminal.scroll_lines(whole),
            WheelRoute::ReportToProgram => {
                let template =
                    pointer_report(terminal::MouseButton::WheelUp, point, event.modifiers);
                self.report_wheel(whole, template, cx);
            },
            WheelRoute::ArrowKeys => self.send_arrows(whole, cx),
        }
        cx.stop_propagation();
        cx.notify();
    }

    pub(crate) fn report_wheel(
        &mut self,
        lines: i32,
        template: Report,
        cx: &mut Context<'_, Self>,
    ) {
        let button = match lines.cmp(&0) {
            Ordering::Greater => terminal::MouseButton::WheelUp,
            _ => terminal::MouseButton::WheelDown,
        };
        for _ in 0..lines.unsigned_abs() {
            let report = Report { button, ..template };
            self.report(MouseAction::Press, report, cx);
        }
    }

    pub(crate) fn send_arrows(
        &mut self,
        lines: i32,
        cx: &mut Context<'_, Self>,
    ) {
        let key = match lines.cmp(&0) {
            Ordering::Greater => "up",
            _ => "down",
        };
        let input = terminal::KeyInput {
            key: key.to_owned(),
            text: None,
            modifiers: terminal::Modifiers::NONE,
        };
        let Some(sequence) = self.terminal.encode_key(&input) else {
            return;
        };
        self.send_bytes(&sequence.repeat(lines.unsigned_abs() as usize), cx);
    }
}

fn grab_offset(
    pointer: f32,
    thumb: ScrollbarThumb,
) -> f32 {
    if (thumb.top..=thumb.top + thumb.length).contains(&pointer) {
        return pointer - thumb.top;
    }
    thumb.length / HALF
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, ScrollDelta, ScrollWheelEvent, TestAppContext, TouchPhase};

    use crate::test_support::{cell_position, feed, open, typed};

    #[gpui::test]
    fn the_wheel_is_reported_to_programs_that_track_the_mouse(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "\x1b[?1000h\x1b[?1006h");
        let position = cell_position(&harness, cx, 2);
        cx.simulate_event(ScrollWheelEvent {
            position,
            delta: ScrollDelta::Lines(gpui::point(0.0, 1.0)),
            modifiers: Modifiers::default(),
            touch_phase: TouchPhase::Moved,
        });
        assert!(
            typed(&harness).starts_with("\x1b[<64;"),
            "{:?}",
            typed(&harness)
        );
    }
}
