//! How one tree row takes part in a drag: source, ghost and drop indicators.

use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, AppContext, Div, DragMoveEvent, InteractiveElement, MouseButton, MouseDownEvent,
    ParentElement, SharedString, Stateful, StatefulInteractiveElement, Styled, Window,
};
use ui::{DragGhost, Palette};

use crate::drag_point::DragPoint;
use crate::drag_press::DragPress;
use crate::dragged_row::DraggedRow;
use crate::drop_line::drop_line;
use crate::drop_mode::DropMode;
use crate::row_action::{RowAction, RowHandler};
use crate::row_id::RowId;

const SOURCE_OPACITY: f32 = 0.4;
const GHOST_WIDTH: f32 = 220.0;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct RowDrag {
    pub(crate) lifted: bool,
    pub(crate) mode: Option<DropMode>,
}

impl RowDrag {
    pub(crate) fn style(
        self,
        shell: Stateful<Div>,
        palette: &Palette,
    ) -> Stateful<Div> {
        let line = self
            .mode
            .and_then(|mode| drop_line(mode, palette.sidebar_drop_line()));
        shell
            .when(self.lifted, |d| d.opacity(SOURCE_OPACITY))
            .when(self.mode == Some(DropMode::Into), |d| {
                d.bg(palette.sidebar_drop_nest())
                    .border_color(palette.sidebar_drop_nest_edge())
            })
            .children(line)
    }
}

pub(crate) fn draggable(
    shell: Stateful<Div>,
    (row, label): (RowId, SharedString),
    handler: &RowHandler,
) -> Stateful<Div> {
    let payload = DraggedRow {
        row: row.clone(),
        label,
    };
    let (pressed, over, dropped) = (
        (row.clone(), Rc::clone(handler)),
        (row.clone(), Rc::clone(handler)),
        (row, Rc::clone(handler)),
    );
    shell
        .on_mouse_down(MouseButton::Left, move |event, window, cx| {
            report_press((&pressed.0, &pressed.1), event, window, cx);
        })
        .on_drag(payload, |dragged, _, _, cx| {
            cx.new(|_| DragGhost::new(dragged.label.clone(), GHOST_WIDTH))
        })
        .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
            report_over((&over.0, &over.1), event, window, cx);
        })
        .on_drop(move |dragged: &DraggedRow, window, cx| {
            let action = RowAction::Dropped(dragged.row.clone(), dropped.0.clone());
            (dropped.1)(action, window, cx);
        })
}

fn report_press(
    (row, handler): (&RowId, &RowHandler),
    event: &MouseDownEvent,
    window: &mut Window,
    cx: &mut App,
) {
    let press = DragPress {
        row: row.clone(),
        at: event.position,
    };
    handler(RowAction::Press(press), window, cx);
}

fn report_over(
    (row, handler): (&RowId, &RowHandler),
    event: &DragMoveEvent<DraggedRow>,
    window: &mut Window,
    cx: &mut App,
) {
    let at = event.event.position;
    if !event.bounds.contains(&at) {
        return;
    }
    let point = DragPoint {
        subject: event.drag(cx).row.clone(),
        target: row.clone(),
        bounds: event.bounds,
        at,
    };
    handler(RowAction::DragOver(point), window, cx);
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, point, px};
    use ui::{Surface, Theme};

    use super::*;
    use crate::row_shell::row_shell;
    use crate::test_support::{mount, recording_handler};

    #[gpui::test]
    fn dragging_a_row_reports_the_press_the_moves_and_the_drop(cx: &mut TestAppContext) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            let shell = row_shell("row".into(), 0, palette).child("label");
            draggable(shell, (RowId::from("r"), "label".into()), &handler)
        });
        let start = point(px(40.0), px(18.0));
        cx.simulate_mouse_down(start, MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(
            point(px(60.0), px(18.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_move(
            point(px(70.0), px(18.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_up(
            point(px(70.0), px(18.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        let seen = seen.borrow();
        let first = seen.first().expect("a press is reported");
        assert!(
            matches!(first, RowAction::Press(press) if press.at == start),
            "press"
        );
        assert!(
            seen.iter().any(|a| matches!(a, RowAction::DragOver(_))),
            "moves"
        );
        let drop = RowAction::Dropped(RowId::from("r"), RowId::from("r"));
        assert!(seen.contains(&drop), "drop");
    }
}
