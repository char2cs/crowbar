use std::time::Duration;

use gpui::{AppContext as _, Entity, TestAppContext};

use crate::display::toast::toast_kind::ToastKind;
use crate::display::toast::toast_queue::ToastContent;
use crate::display::toast::toast_queue::ToastId;
use crate::display::toast::toast_queue::ToastLifetime;
use crate::display::toast::toast_queue::ToastPlacement;
use crate::display::toast::toast_queue::ToastQueue;

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
