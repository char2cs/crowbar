//! Mouse handling of the view: selection, reporting to the program, links, scrollbar and wheel.

use gpui::{
    AsyncApp, ClipboardItem, Context, CursorStyle, MouseButton, MouseDownEvent, MouseMoveEvent,
    MouseUpEvent, Pixels, Point, ScrollDelta, ScrollWheelEvent, WeakEntity, Window,
};
use terminal::{
    CellHit, DragEdge, Highlight, LinkTarget, MouseAction, MouseEvent, ScrollbarThumb, ViewPoint,
    WheelRoute, resolve_file_path, route_wheel,
};
use terminal::{ClientMessage, MouseTracking};

use crate::auto_scroll::AutoScroll;
use crate::drag_state::DragState;
use crate::key_conversion::terminal_modifiers;
use crate::pointer_report::PointerReport;
use crate::terminal_view::TerminalView;
use crate::terminal_view_event::TerminalViewEvent;

const WHEEL_LINES_PER_NOTCH: f32 = 3.0;
const HALF: f32 = 2.0;
const AUTO_SCROLL_INTERVAL: std::time::Duration = std::time::Duration::from_millis(50);

impl TerminalView {
    pub(crate) fn pointer_style(&self) -> CursorStyle {
        match self.hovered.is_some() {
            true => CursorStyle::PointingHand,
            false => CursorStyle::IBeam,
        }
    }

    fn hit_at(
        &self,
        position: Point<Pixels>,
    ) -> Option<CellHit> {
        let layout = self.layout?;
        let (x, y) = layout.local(position);
        Some(layout.metrics.hit(x, y))
    }

    fn program_tracks_mouse(
        &self,
        modifiers: gpui::Modifiers,
    ) -> bool {
        self.terminal.mouse_tracking() != MouseTracking::Off && !modifiers.shift
    }

    pub(crate) fn on_mouse_down(
        &mut self,
        button: MouseButton,
        event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
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
            self.report_press(button, hit.point, event.modifiers, cx);
            return;
        }
        self.begin_selection_or_paste(button, hit, cx);
    }

    fn report_press(
        &mut self,
        button: MouseButton,
        point: ViewPoint,
        modifiers: gpui::Modifiers,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(reported) = reported_button(button) else {
            return;
        };
        self.drag = DragState::ReportingToProgram(reported);
        let report = pointer_report(reported, point, modifiers);
        self.report(MouseAction::Press, report, cx);
    }

    fn report(
        &mut self,
        action: MouseAction,
        report: PointerReport,
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

    fn begin_selection_or_paste(
        &mut self,
        button: MouseButton,
        hit: CellHit,
        cx: &mut Context<'_, Self>,
    ) {
        match button {
            MouseButton::Left => {
                let kind = self.clicks.press(hit.point, self.clock.elapsed());
                self.terminal.begin_selection(hit.point, hit.side, kind);
                self.drag = DragState::Selecting;
                cx.notify();
            },
            MouseButton::Middle => self.paste_primary(cx),
            _ => {},
        }
    }

    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    fn paste_primary(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(text) = cx.read_from_primary().and_then(|item| item.text()) {
            self.paste(&text, cx);
        }
    }

    #[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
    fn paste_primary(
        &mut self,
        _: &mut Context<'_, Self>,
    ) {
    }

    pub(crate) fn on_mouse_move(
        &mut self,
        event: &MouseMoveEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(hit) = self.hit_at(event.position) else {
            return;
        };
        match self.drag {
            DragState::Selecting => self.extend_selection(hit, cx),
            DragState::ReportingToProgram(button) => self.report_drag(button, hit.point, event, cx),
            DragState::ScrollbarThumb { grab } => self.drag_thumb(event.position, grab, cx),
            DragState::Idle => self.hover(hit.point, event, cx),
        }
    }

    fn report_drag(
        &mut self,
        button: terminal::MouseButton,
        point: ViewPoint,
        event: &MouseMoveEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let report = pointer_report(button, point, event.modifiers);
        self.report(MouseAction::Drag, report, cx);
    }

    fn hover(
        &mut self,
        point: ViewPoint,
        event: &MouseMoveEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if self.program_tracks_mouse(event.modifiers) {
            let report = pointer_report(terminal::MouseButton::None, point, event.modifiers);
            self.report(MouseAction::Move, report, cx);
        }
        let link = link_modifier_held(event.modifiers)
            .then(|| self.terminal.link_at(point))
            .flatten();
        if link != self.hovered {
            self.hovered = link;
            cx.notify();
        }
    }

    fn extend_selection(
        &mut self,
        hit: CellHit,
        cx: &mut Context<'_, Self>,
    ) {
        self.terminal.extend_selection(hit.point, hit.side);
        self.track_edge(hit.edge, cx);
        cx.notify();
    }

    fn track_edge(
        &mut self,
        edge: DragEdge,
        cx: &mut Context<'_, Self>,
    ) {
        if edge == DragEdge::Inside {
            self.auto_scroll = None;
            return;
        }
        if self
            .auto_scroll
            .as_ref()
            .is_some_and(|running| running.edge == edge)
        {
            return;
        }
        let task = cx.spawn(async move |view, cx| {
            scroll_loop(view, edge, cx).await;
        });
        self.auto_scroll = Some(AutoScroll { edge, _task: task });
    }

    fn auto_scroll_step(
        &mut self,
        edge: DragEdge,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(layout) = self.layout else {
            return;
        };
        let (lines, row) = match edge {
            DragEdge::Above => (1, 0),
            _ => (-1, layout.metrics.lines.saturating_sub(1)),
        };
        self.terminal.scroll_lines(lines);
        let column = match row {
            0 => 0,
            _ => layout.metrics.columns,
        };
        self.terminal
            .extend_selection(ViewPoint { row, column }, terminal::CellSide::Right);
        cx.notify();
    }

    pub(crate) fn on_mouse_up(
        &mut self,
        button: MouseButton,
        event: &MouseUpEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let drag = std::mem::replace(&mut self.drag, DragState::Idle);
        self.auto_scroll = None;
        match drag {
            DragState::Selecting if button == MouseButton::Left => self.finish_selection(cx),
            DragState::ReportingToProgram(reported) => self.report_release(reported, event, cx),
            _ => {},
        }
    }

    fn report_release(
        &mut self,
        button: terminal::MouseButton,
        event: &MouseUpEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let point = self
            .hit_at(event.position)
            .map_or_else(ViewPoint::default, |hit| hit.point);
        let report = pointer_report(button, point, event.modifiers);
        self.report(MouseAction::Release, report, cx);
    }

    fn finish_selection(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let selected = self.terminal.finish_selection();
        cx.notify();
        Self::publish_primary(selected, cx);
    }

    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    fn publish_primary(
        text: Option<String>,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(text) = text {
            cx.write_to_primary(ClipboardItem::new_string(text));
        }
    }

    #[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
    fn publish_primary(
        _: Option<String>,
        _: &mut Context<'_, Self>,
    ) {
    }

    fn open_link(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let Some(hit) = self.hit_at(position) else {
            return false;
        };
        let Some(link) = self.terminal.link_at(hit.point) else {
            return false;
        };
        match link.target {
            LinkTarget::Url(url) => cx.emit(TerminalViewEvent::OpenUrl { url, position }),
            LinkTarget::File { path, line } => self.open_file(&path, line, cx),
        }
        true
    }

    fn open_file(
        &mut self,
        path: &str,
        line: Option<u32>,
        cx: &mut Context<'_, Self>,
    ) {
        match resolve_file_path(path, self.working_directory.as_deref()) {
            Some(path) => cx.emit(TerminalViewEvent::OpenFile { path, line }),
            None => cx.emit(TerminalViewEvent::UnresolvedFile(path.to_owned())),
        }
    }

    fn scrollbar_thumb(&self) -> Option<ScrollbarThumb> {
        let layout = self.layout?;
        let snapshot = self.terminal.snapshot();
        ScrollbarThumb::new(
            f32::from(layout.track.size.height),
            layout.metrics.lines,
            snapshot.history_size,
            snapshot.display_offset,
        )
    }

    fn begin_thumb_drag(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(layout), Some(thumb)) = (self.layout, self.scrollbar_thumb()) else {
            return;
        };
        let pointer = f32::from(position.y - layout.track.origin.y);
        let on_thumb = pointer >= thumb.top && pointer <= thumb.top + thumb.length;
        let grab = match on_thumb {
            true => pointer - thumb.top,
            false => thumb.length / HALF,
        };
        self.drag = DragState::ScrollbarThumb { grab };
        self.drag_thumb(position, grab, cx);
    }

    fn drag_thumb(
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
            WheelRoute::ReportToProgram => self.report_wheel(whole, point, event.modifiers, cx),
            WheelRoute::ArrowKeys => self.send_arrows(whole, cx),
        }
        cx.stop_propagation();
        cx.notify();
    }

    fn report_wheel(
        &mut self,
        lines: i32,
        point: ViewPoint,
        modifiers: gpui::Modifiers,
        cx: &mut Context<'_, Self>,
    ) {
        let button = match lines > 0 {
            true => terminal::MouseButton::WheelUp,
            false => terminal::MouseButton::WheelDown,
        };
        for _ in 0..lines.unsigned_abs() {
            let report = pointer_report(button, point, modifiers);
            self.report(MouseAction::Press, report, cx);
        }
    }

    fn send_arrows(
        &mut self,
        lines: i32,
        cx: &mut Context<'_, Self>,
    ) {
        let key = match lines > 0 {
            true => "up",
            false => "down",
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

    pub(crate) fn hovered_cells(&self) -> &[Highlight] {
        self.hovered
            .as_ref()
            .map_or(&[], |link| link.cells.as_slice())
    }
}

fn reported_button(button: MouseButton) -> Option<terminal::MouseButton> {
    match button {
        MouseButton::Left => Some(terminal::MouseButton::Left),
        MouseButton::Middle => Some(terminal::MouseButton::Middle),
        MouseButton::Right => Some(terminal::MouseButton::Right),
        MouseButton::Navigate(_) => None,
    }
}

pub(crate) fn link_modifier_held(modifiers: gpui::Modifiers) -> bool {
    match cfg!(target_os = "macos") {
        true => modifiers.platform,
        false => modifiers.control,
    }
}

fn pointer_report(
    button: terminal::MouseButton,
    point: ViewPoint,
    modifiers: gpui::Modifiers,
) -> PointerReport {
    PointerReport {
        button,
        point,
        modifiers: terminal_modifiers(modifiers),
    }
}

async fn scroll_loop(
    view: WeakEntity<TerminalView>,
    edge: DragEdge,
    cx: &mut AsyncApp,
) -> Option<()> {
    loop {
        cx.background_executor().timer(AUTO_SCROLL_INTERVAL).await;
        view.update(cx, |view, cx| view.auto_scroll_step(edge, cx))
            .ok()?;
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, MouseDownEvent, MouseUpEvent, TestAppContext};

    use crate::terminal_view::TerminalView;
    use crate::terminal_view_event::TerminalViewEvent;
    use crate::test_support::{cell_position, feed, open, record_events, typed};

    fn press(
        position: gpui::Point<gpui::Pixels>,
        modifiers: Modifiers,
    ) -> MouseDownEvent {
        MouseDownEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 1,
            first_mouse: false,
        }
    }

    fn release(
        position: gpui::Point<gpui::Pixels>,
        modifiers: Modifiers,
    ) -> MouseUpEvent {
        MouseUpEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 1,
        }
    }

    #[gpui::test]
    fn a_modifier_click_on_a_url_asks_the_host_to_open_it(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "see https://x.dev/a now");
        let events = record_events::<TerminalView, TerminalViewEvent>(cx, &harness.view);
        let position = cell_position(&harness, cx, 8);
        let modifiers = Modifiers {
            control: true,
            platform: true,
            ..Modifiers::default()
        };
        cx.simulate_event(press(position, modifiers));
        let opened = events.borrow().iter().any(|event| matches!(event, TerminalViewEvent::OpenUrl { url, .. } if url == "https://x.dev/a"));
        assert!(opened, "{:?}", events.borrow());
    }

    #[gpui::test]
    fn dragging_selects_text_and_a_plain_click_does_not(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "select me");
        let (from, to) = (
            cell_position(&harness, cx, 0),
            cell_position(&harness, cx, 6),
        );
        cx.simulate_event(press(from, Modifiers::default()));
        cx.simulate_mouse_move(to, None, Modifiers::default());
        cx.simulate_event(release(to, Modifiers::default()));
        let selected = harness.view.read_with(cx, |view, _| view.selected_text());
        assert_eq!(selected.as_deref(), Some("select"));
        let elsewhere = cell_position(&harness, cx, 8);
        cx.simulate_event(press(elsewhere, Modifiers::default()));
        cx.simulate_event(release(elsewhere, Modifiers::default()));
        assert!(
            harness
                .view
                .read_with(cx, |view, _| view.selected_text())
                .is_none()
        );
    }

    #[gpui::test]
    fn programs_that_track_the_mouse_receive_reports(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "\x1b[?1000h\x1b[?1006h");
        let position = cell_position(&harness, cx, 2);
        cx.simulate_event(press(position, Modifiers::default()));
        cx.simulate_event(release(position, Modifiers::default()));
        assert_eq!(typed(&harness), "\x1b[<0;3;1M\x1b[<0;3;1m");
    }
}
