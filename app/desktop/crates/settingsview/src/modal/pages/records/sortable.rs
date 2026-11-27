use gpui::prelude::FluentBuilder as _;
use gpui::{
    AppContext as _, Div, InteractiveElement, IntoElement, Stateful, StatefulInteractiveElement,
    Styled,
};

use crate::frame::Frame;
use crate::modal::pages::records::drag_record::DragRecord;
use crate::modal::pages::records::hover::Hover;
use crate::modal::pages::records::listed_record::ListedRecord;

const LIFTED_OPACITY: f32 = 0.4;

pub(crate) fn sortable(
    row: Stateful<Div>,
    frame: &Frame<'_>,
    entry: ListedRecord<'_>,
) -> impl IntoElement {
    let ListedRecord {
        source,
        record,
        index,
    } = entry;
    let drag = DragRecord {
        source: source.to_owned(),
        record: record.id().to_owned(),
    };
    let lifted = frame.lift.is_some_and(|lift| lift.holds(source, record));
    let hover = Hover {
        sink: frame.sink.clone(),
        shown: frame.lift.cloned(),
        index,
    };
    let drop = frame.sink.clone();
    let target = source.to_owned();
    row.cursor_grab()
        .when(lifted, |row| row.opacity(LIFTED_OPACITY))
        .on_drag(drag, |_, _, _, cx| cx.new(|_| gpui::EmptyView))
        .on_drag_move::<DragRecord>(move |event, window, cx| hover.moved(event, (window, cx)))
        .on_drop::<DragRecord>(move |drag, window, cx| {
            drag.deliver(&drop, (&target, index), (window, cx));
        })
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, VisualTestContext, point, px};
    use settings::{AGENT_RECORDS, RecordCommand};

    use crate::test_support::open;

    #[gpui::test]
    fn a_lifted_row_takes_the_slot_under_the_pointer_and_the_others_shift(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "providers");
        let top = |cx: &mut VisualTestContext, id: &str| {
            cx.debug_bounds(Box::leak(
                format!("record-{AGENT_RECORDS}-{id}").into_boxed_str(),
            ))
            .unwrap()
        };
        let (claude, codex) = (top(cx, "claude"), top(cx, "codex"));
        let none = Modifiers::none();
        let grip = point(claude.left() + px(12.0), claude.center().y);
        cx.simulate_mouse_down(grip, MouseButton::Left, none);
        for dy in [4.0, 12.0] {
            cx.simulate_mouse_move(point(grip.x, grip.y + px(dy)), MouseButton::Left, none);
        }
        let over = point(grip.x, codex.center().y);
        cx.simulate_mouse_move(over, MouseButton::Left, none);
        cx.run_until_parked();
        assert!(
            top(cx, "claude").top() > top(cx, "codex").top(),
            "claude {:?} still above codex {:?}",
            top(cx, "claude"),
            top(cx, "codex")
        );
        cx.simulate_mouse_up(over, MouseButton::Left, none);
        let (_, record, command) = harness.backend.commands().pop().unwrap();
        assert_eq!(
            (record.as_str(), command),
            ("claude", RecordCommand::Move { to: 1 })
        );
    }
}
