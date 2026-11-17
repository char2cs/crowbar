use std::time::Duration;

use gpui::{AppContext as _, Entity, TestAppContext};

use crate::toast_content::ToastContent;
use crate::toast_id::ToastId;
use crate::toast_kind::ToastKind;
use crate::toast_lifetime::ToastLifetime;
use crate::toast_placement::ToastPlacement;
use crate::toast_queue::ToastQueue;

pub(crate) fn queue(cx: &mut TestAppContext) -> Entity<ToastQueue> {
    cx.new(|_| ToastQueue::new(ToastPlacement::default()))
}

pub(crate) fn push(
    queue: &Entity<ToastQueue>,
    lifetime: ToastLifetime,
    cx: &mut TestAppContext,
) -> ToastId {
    queue.update(cx, |queue, cx| {
        queue.push(ToastContent::new(ToastKind::Info, "Saved"), lifetime, cx)
    })
}

pub(crate) fn advance(
    cx: &mut TestAppContext,
    by: Duration,
) {
    cx.executor().advance_clock(by);
    cx.run_until_parked();
}
