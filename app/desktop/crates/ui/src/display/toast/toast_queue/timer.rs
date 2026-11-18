use std::time::Duration;

use gpui::{Context, Task};

use crate::display::toast::toast_queue::ToastQueue;
use crate::display::toast::toast_queue::toast_id::ToastId;

impl ToastQueue {
    pub(crate) fn expiry(
        id: ToastId,
        timeout: Option<Duration>,
        cx: &mut Context<'_, Self>,
    ) -> Option<Task<()>> {
        timeout.map(|delay| Self::after(delay, id, cx, Self::dismiss))
    }

    pub(crate) fn after(
        delay: Duration,
        id: ToastId,
        cx: &mut Context<'_, Self>,
        then: fn(&mut Self, ToastId, &mut Context<'_, Self>),
    ) -> Task<()> {
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(delay).await;
            note_dropped(this.update(cx, |queue, cx| then(queue, id, cx)));
        })
    }
}

fn note_dropped<Failure: std::fmt::Display>(update: Result<(), Failure>) {
    if let Err(error) = update {
        tracing::debug!(%error, "toast queue dropped before its timer fired");
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;

    use crate::display::toast::test_support::{advance, push, queue};
    use crate::display::toast::toast_queue::toast_id::ToastId;
    use crate::display::toast::toast_queue::toast_lifetime::ToastLifetime;
    use crate::foundation::tokens::timing;

    #[gpui::test]
    fn an_automatic_toast_expires_after_five_seconds_and_a_pinned_one_stays(
        cx: &mut TestAppContext
    ) {
        let queue = queue(cx);
        push(&queue, ToastLifetime::Auto, cx);
        push(&queue, ToastLifetime::Pinned, cx);
        advance(
            cx,
            timing::TOAST_TIMEOUT.saturating_sub(Duration::from_millis(1)),
        );
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 2);
        advance(cx, Duration::from_millis(1) + timing::SLOW);
        let left = queue.read_with(cx, |queue, _| {
            queue.entries.iter().map(|e| e.id).collect::<Vec<_>>()
        });
        assert_eq!(left, [ToastId(1)]);
        advance(cx, Duration::from_secs(600));
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 1);
    }
}
