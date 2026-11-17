use gpui::{ClipboardItem, Context, MouseButton, MouseUpEvent, Pixels, Point};
use terminal::{LinkTarget, MouseAction, ViewPoint, resolve_file_path};

use crate::button_event::ButtonEvent;
use crate::drag_state::DragState;
use crate::pointer_report::pointer_report;
use crate::terminal_view::TerminalView;
use crate::terminal_view_event::TerminalViewEvent;

impl TerminalView {
    pub(crate) fn on_mouse_up(
        &mut self,
        release: ButtonEvent<'_, MouseUpEvent>,
        cx: &mut Context<'_, Self>,
    ) {
        let ButtonEvent { button, event } = release;
        let drag = std::mem::replace(&mut self.drag, DragState::Idle);
        self.auto_scroll = None;
        match drag {
            DragState::Selecting if button == MouseButton::Left => self.finish_selection(cx),
            DragState::ReportingToProgram(reported) => self.report_release(reported, event, cx),
            _ => {},
        }
    }

    pub(crate) fn report_release(
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

    pub(crate) fn finish_selection(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let selected = self.terminal.finish_selection();
        cx.notify();
        Self::publish_primary(selected, cx);
    }

    pub(crate) fn open_link(
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

    pub(crate) fn open_file(
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

    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    pub(crate) fn publish_primary(
        text: Option<String>,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(text) = text {
            cx.write_to_primary(ClipboardItem::new_string(text));
        }
    }

    #[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
    pub(crate) fn publish_primary(
        _: Option<String>,
        _: &mut Context<'_, Self>,
    ) {
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext};

    use crate::test_support::{cell_position, feed, open, press, release, typed};

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
