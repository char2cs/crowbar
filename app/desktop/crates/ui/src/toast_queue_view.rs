//! Paints a [`ToastQueue`] as a column of toasts at its placement.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    Context, Div, Entity, InteractiveElement as _, IntoElement, MouseButton, MouseDownEvent,
    MouseMoveEvent, MouseUpEvent, ParentElement as _, Render, Stateful, Styled as _, Window, div,
    px,
};

use crate::close_handler::CloseHandler;
use crate::toast_id::ToastId;

use crate::spacing;
use crate::toast::Toast;
use crate::toast_entry::ToastEntry;
use crate::toast_placement::ToastPlacement;
use crate::toast_queue::ToastQueue;
use crate::toast_selection::visible;
use crate::toast_side::ToastSide;

const FLOATING_WIDTH: f32 = 288.0;
const FLOATING_INSET: f32 = 16.0;

fn column(placement: ToastPlacement) -> Div {
    let base = div().absolute().flex().gap(spacing::STEP_2);
    match placement {
        ToastPlacement::Docked(_) => base
            .left_0()
            .right_0()
            .bottom_0()
            .p(spacing::STEP_2)
            .flex_col_reverse(),
        ToastPlacement::Floating(side) => base
            .w(px(FLOATING_WIDTH))
            .bottom(px(FLOATING_INSET))
            .flex_col()
            .when(side == ToastSide::Left, |column| {
                column.left(px(FLOATING_INSET))
            })
            .when(side == ToastSide::Right, |column| {
                column.right(px(FLOATING_INSET))
            }),
    }
}

fn close_handler(
    queue: Entity<ToastQueue>,
    id: ToastId,
) -> CloseHandler {
    Rc::new(move |_, cx| queue.update(cx, |queue, cx| queue.dismiss(id, cx)))
}

fn card(
    entry: &ToastEntry,
    cx: &mut Context<'_, ToastQueue>,
) -> Stateful<Div> {
    let id = entry.id;
    let toast = Toast::new(("toast", id.0), entry.kind, entry.message.clone())
        .updated(entry.updates)
        .full_width()
        .on_close(close_handler(cx.entity(), id))
        .when(entry.leaving, Toast::leaving);
    let start = cx.listener(move |queue, event: &MouseDownEvent, _, _| {
        queue.begin_swipe(id, event.position);
    });
    let end = cx.listener(|queue, _: &MouseUpEvent, _, _| queue.swipe = None);
    let ended = cx.listener(|queue, _: &MouseUpEvent, _, _| queue.swipe = None);
    let drag = cx.listener(|queue, event: &MouseMoveEvent, _, cx| {
        queue.drag_swipe(event.position, cx);
    });
    div()
        .id(("toast-swipe", id.0))
        .w_full()
        .on_mouse_down(MouseButton::Left, start)
        .on_mouse_move(drag)
        .on_mouse_up(MouseButton::Left, end)
        .on_mouse_up_out(MouseButton::Left, ended)
        .child(toast)
}

impl Render for ToastQueue {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let shown: Vec<_> = visible(&self.entries, self.placement.limit())
            .into_iter()
            .map(|entry| card(entry, cx))
            .collect();
        column(self.placement).children(shown)
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::{AppContext as _, Entity, Modifiers, MouseButton, TestAppContext, point};

    use super::*;
    use crate::test_support::open;
    use crate::toast_content::ToastContent;
    use crate::toast_kind::ToastKind;
    use crate::toast_lifetime::ToastLifetime;
    use crate::toast_placement::ToastPlacement;

    const WIDTH: f32 = 272.0;
    const HEIGHT: f32 = 300.0;
    const CARD: f32 = 46.0;
    const GAP: f32 = 8.0;

    fn stacked(
        count: usize,
        cx: &mut TestAppContext,
    ) -> Entity<ToastQueue> {
        let queue = cx.new(|_| ToastQueue::new(ToastPlacement::Docked(ToastSide::Left)));
        for index in 0..count {
            let content = ToastContent::new(ToastKind::Info, format!("Toast {index}"));
            queue.update(cx, |queue, cx| queue.push(content, ToastLifetime::Auto, cx));
        }
        queue
    }

    fn show<'a>(
        queue: &Entity<ToastQueue>,
        cx: &'a mut TestAppContext,
    ) -> &'a mut gpui::VisualTestContext {
        let queue = queue.clone();
        open(cx, move |_, _| {
            div()
                .relative()
                .w(px(WIDTH))
                .h(px(HEIGHT))
                .child(queue.clone())
        })
    }

    fn close_button(from_bottom: f32) -> gpui::Point<gpui::Pixels> {
        let top = HEIGHT - GAP - CARD - from_bottom * (CARD + GAP);
        point(px(WIDTH - GAP - 22.0), px(top + 22.0))
    }

    fn leaving(
        queue: &Entity<ToastQueue>,
        cx: &mut gpui::VisualTestContext,
    ) -> Vec<bool> {
        queue.read_with(cx, |queue, _| {
            queue.entries.iter().map(|e| e.leaving).collect()
        })
    }

    #[gpui::test]
    fn a_docked_column_shows_three_toasts_newest_at_the_bottom(cx: &mut TestAppContext) {
        let queue = stacked(5, cx);
        let cx = show(&queue, cx);
        cx.simulate_click(close_button(2.0), Modifiers::none());
        assert_eq!(leaving(&queue, cx), [false, false, true, false, false]);
        cx.simulate_click(close_button(3.0), Modifiers::none());
        assert_eq!(leaving(&queue, cx), [false, false, true, false, false]);
    }

    #[gpui::test]
    fn the_close_button_fades_the_toast_and_the_queue_then_removes_it(cx: &mut TestAppContext) {
        let queue = stacked(1, cx);
        let cx = show(&queue, cx);
        cx.simulate_click(close_button(0.0), Modifiers::none());
        assert_eq!(leaving(&queue, cx), [true]);
        cx.executor().advance_clock(Duration::from_millis(250));
        cx.run_until_parked();
        assert!(queue.read_with(cx, |queue, _| queue.is_empty()));
    }

    #[gpui::test]
    fn dragging_a_toast_toward_the_sidebar_edge_dismisses_it(cx: &mut TestAppContext) {
        let queue = stacked(1, cx);
        let cx = show(&queue, cx);
        let start = point(px(150.0), px(HEIGHT - GAP - CARD / 2.0));
        cx.simulate_mouse_down(start, MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(
            start - point(px(20.0), px(0.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        assert_eq!(leaving(&queue, cx), [false]);
        cx.simulate_mouse_move(
            start - point(px(50.0), px(0.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        assert_eq!(leaving(&queue, cx), [true]);
    }
}
