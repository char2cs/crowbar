//! Makes a tree row pressable, draggable and a drop target.

use std::rc::Rc;

use gpui::{
    App, AppContext, Div, DragMoveEvent, InteractiveElement, MouseButton, MouseDownEvent, Stateful,
    StatefulInteractiveElement, Window,
};
use sidebar::RowId;
use ui::DraggedRow;

use crate::panels::chats::reorder::row_ghost::{GhostBuild, RowGhost};
use crate::sidebar::DragPoint;
use crate::sidebar::DragPress;
use crate::sidebar::{RowAction, RowHandler};

type Reporter = (RowId, RowHandler);

pub(crate) fn draggable(
    shell: Stateful<Div>,
    payload: DraggedRow,
    ghost: GhostBuild,
    handler: &RowHandler,
) -> Stateful<Div> {
    let reporter = || (RowId::from(payload.row.as_ref()), Rc::clone(handler));
    let (pressed, over, dropped) = (reporter(), reporter(), reporter());
    shell
        .on_mouse_down(MouseButton::Left, move |event, window, cx| {
            report_press(&pressed, event, window, cx);
        })
        .on_drag(payload, move |_, _, _, cx| {
            cx.new(|_| RowGhost::new(Rc::clone(&ghost)))
        })
        .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
            report_over(&over, event, window, cx);
        })
        .on_drop(move |dragged: &DraggedRow, window, cx| {
            report_drop(&dropped, dragged, window, cx);
        })
}

fn report_press(
    reporter: &Reporter,
    event: &MouseDownEvent,
    window: &mut Window,
    cx: &mut App,
) {
    let (row, handler) = reporter;
    let press = DragPress {
        row: row.clone(),
        at: event.position,
    };
    handler(RowAction::Press(press), window, cx);
}

fn report_drop(
    reporter: &Reporter,
    dragged: &DraggedRow,
    window: &mut Window,
    cx: &mut App,
) {
    let (row, handler) = reporter;
    handler(
        RowAction::Dropped(RowId::from(dragged.row.as_ref()), row.clone()),
        window,
        cx,
    );
}

fn report_over(
    reporter: &Reporter,
    event: &DragMoveEvent<DraggedRow>,
    window: &mut Window,
    cx: &mut App,
) {
    let (row, handler) = reporter;
    let at = event.event.position;
    if !event.bounds.contains(&at) {
        return;
    }
    let point = DragPoint {
        subject: RowId::from(event.drag(cx).row.as_ref()),
        target: row.clone(),
        bounds: event.bounds,
        at,
    };
    handler(RowAction::DragOver(point), window, cx);
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use gpui::{IntoElement, Modifiers, ParentElement, TestAppContext, point, px};
    use ui::{Surface, Theme};

    use super::*;
    use crate::panels::chats::parts::row_shell;
    use crate::test_support::{mount, recording_handler};

    fn payload() -> DraggedRow {
        DraggedRow {
            row: Arc::from("r"),
            chat: None,
        }
    }

    fn ghost() -> GhostBuild {
        Rc::new(|_| gpui::div().into_any_element())
    }

    #[gpui::test]
    fn dragging_a_row_reports_the_press_the_moves_and_the_drop(cx: &mut TestAppContext) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            let shell = row_shell("row".into(), 0, palette).child("label");
            draggable(shell, payload(), ghost(), &handler)
        });
        let none = Modifiers::none();
        let start = point(px(40.0), px(18.0));
        cx.simulate_mouse_down(start, MouseButton::Left, none);
        for x in [60.0, 70.0] {
            cx.simulate_mouse_move(point(px(x), px(18.0)), MouseButton::Left, none);
        }
        cx.simulate_mouse_up(point(px(70.0), px(18.0)), MouseButton::Left, none);
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
