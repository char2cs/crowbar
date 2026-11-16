//! Toast queues docked in a sidebar-sized column and floating at the window corner.

use gpui::{
    App, AppContext as _, Context, Entity, IntoElement, ParentElement, Pixels, Styled, div, px,
};
use ui::{ToastContent, ToastKind, ToastLifetime, ToastPlacement, ToastQueue, ToastSide, spacing};

const DOCK_WIDTH: Pixels = px(272.0);
const DOCK_HEIGHT: Pixels = px(300.0);
const FLOAT_WIDTH: Pixels = px(320.0);

const DOCKED: [(ToastKind, &str, ToastLifetime); 4] = [
    (
        ToastKind::Error,
        "Could not reach the daemon",
        ToastLifetime::Pinned,
    ),
    (ToastKind::Success, "Branch created", ToastLifetime::Pinned),
    (ToastKind::Info, "Merged into main", ToastLifetime::Pinned),
    (
        ToastKind::Loading,
        "Creating the workspace",
        ToastLifetime::Pinned,
    ),
];

fn queue(
    cx: &mut App,
    placement: ToastPlacement,
    toasts: &[(ToastKind, &'static str, ToastLifetime)],
) -> Entity<ToastQueue> {
    let queue = cx.new(|_| ToastQueue::new(placement));
    queue.update(cx, |queue, cx| push_all(queue, toasts, cx));
    queue
}

fn push_all(
    queue: &mut ToastQueue,
    toasts: &[(ToastKind, &'static str, ToastLifetime)],
    cx: &mut Context<'_, ToastQueue>,
) {
    for &(kind, message, lifetime) in toasts.iter().rev() {
        queue.push(ToastContent::new(kind, message), lifetime, cx);
    }
}

pub(crate) fn queues(cx: &mut App) -> Vec<Entity<ToastQueue>> {
    vec![
        queue(cx, ToastPlacement::Docked(ToastSide::Left), &DOCKED),
        queue(cx, ToastPlacement::Floating(ToastSide::Left), &DOCKED[..2]),
    ]
}

fn stage(
    width: Pixels,
    queue: &Entity<ToastQueue>,
) -> gpui::Div {
    div()
        .relative()
        .w(width)
        .h(DOCK_HEIGHT)
        .border_1()
        .border_dashed()
        .child(queue.clone())
}

pub(crate) fn toasts(queues: &[Entity<ToastQueue>]) -> impl IntoElement {
    div()
        .flex()
        .gap(spacing::STEP_6)
        .children(queues.first().map(|queue| stage(DOCK_WIDTH, queue)))
        .children(queues.get(1).map(|queue| stage(FLOAT_WIDTH, queue)))
}
