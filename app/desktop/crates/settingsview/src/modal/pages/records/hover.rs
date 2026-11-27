use gpui::{App, DragMoveEvent, Window};

use crate::intent::Intent;
use crate::intent::Sink;
use crate::lift::Lift;
use crate::modal::pages::records::drag_record::DragRecord;

pub(crate) struct Hover {
    pub(crate) sink: Sink,
    pub(crate) shown: Option<Lift>,
    pub(crate) index: usize,
}

impl Hover {
    pub(crate) fn moved(
        &self,
        event: &DragMoveEvent<DragRecord>,
        (window, cx): (&mut Window, &mut App),
    ) {
        if !event.bounds.contains(&event.event.position) {
            return;
        }
        let held = event.drag(cx);
        let lift = Lift {
            source: held.source.clone(),
            record: held.record.clone(),
            over: self.index,
        };
        if self.shown.as_ref() == Some(&lift) {
            return;
        }
        self.sink.deliver(Intent::Lift(lift), (window, cx));
    }
}

#[cfg(test)]
mod tests {
    use gpui::{
        Bounds, Modifiers, MouseButton, Pixels, TestAppContext, VisualTestContext, point, px,
    };
    use settings::AGENT_RECORDS;

    use crate::test_support::open;

    fn bounds(
        cx: &mut VisualTestContext,
        id: &str,
    ) -> Bounds<Pixels> {
        let selector = format!("record-{AGENT_RECORDS}-{id}");
        cx.debug_bounds(Box::leak(selector.into_boxed_str()))
            .unwrap()
    }

    #[gpui::test]
    fn a_drag_that_leaves_the_rows_keeps_the_slot_it_last_offered(cx: &mut TestAppContext) {
        let (_harness, cx) = open(cx, "providers");
        let (claude, codex) = (bounds(cx, "claude"), bounds(cx, "codex"));
        let none = Modifiers::none();
        let grip = point(claude.left() + px(12.0), claude.center().y);
        cx.simulate_mouse_down(grip, MouseButton::Left, none);
        cx.simulate_mouse_move(point(grip.x, grip.y + px(8.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(grip.x, codex.center().y), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(4.0), px(4.0)), MouseButton::Left, none);
        cx.run_until_parked();
        assert!(bounds(cx, "claude").top() > bounds(cx, "codex").top());
    }
}
