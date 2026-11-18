use gpui::{AsyncApp, Context, MouseMoveEvent, WeakEntity, Window};
use terminal::{CellHit, DragEdge, Highlight, MouseAction, ViewPoint};

use crate::terminal_view::TerminalView;
use crate::terminal_view::input::key_conversion::link_modifier_held;
use crate::terminal_view::input::pointer::auto_scroll::AutoScroll;
use crate::terminal_view::input::pointer::drag_state::DragState;
use crate::terminal_view::input::pointer::pointer_report::{PointerReport, pointer_report};

const AUTO_SCROLL_INTERVAL: std::time::Duration = std::time::Duration::from_millis(50);

impl TerminalView {
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
            DragState::ReportingToProgram(button) => {
                self.report_drag(pointer_report(button, hit.point, event.modifiers), cx);
            },
            DragState::ScrollbarThumb { grab } => self.drag_thumb(event.position, grab, cx),
            DragState::Idle => self.hover(hit.point, event, cx),
        }
    }

    pub(crate) fn report_drag(
        &mut self,
        report: PointerReport,
        cx: &mut Context<'_, Self>,
    ) {
        self.report(MouseAction::Drag, report, cx);
    }

    pub(crate) fn hover(
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

    pub(crate) fn extend_selection(
        &mut self,
        hit: CellHit,
        cx: &mut Context<'_, Self>,
    ) {
        self.terminal.extend_selection(hit.point, hit.side);
        self.track_edge(hit.edge, cx);
        cx.notify();
    }

    pub(crate) fn track_edge(
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

    pub(crate) fn auto_scroll_step(
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

    pub(crate) fn hovered_cells(&self) -> &[Highlight] {
        self.hovered
            .as_ref()
            .map_or(&[], |link| link.cells.as_slice())
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
    use gpui::{Modifiers, TestAppContext};

    use crate::test_support::{cell_position, feed, open, press, release};

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
}
