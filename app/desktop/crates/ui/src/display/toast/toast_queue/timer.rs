pub(super) mod toast_hold;

use std::time::{Duration, Instant};

use gpui::{Context, Task, Window};

use crate::display::toast::toast_queue::ToastQueue;
use crate::display::toast::toast_queue::entry::Entry;
use crate::display::toast::toast_queue::timer::toast_hold::ToastHold;
use crate::display::toast::toast_queue::toast_id::ToastId;
use crate::foundation::Pose;
use crate::foundation::WindowActivity;

pub(crate) type Expiry = (Option<Task<()>>, Option<Instant>, Option<Duration>);

impl ToastQueue {
    pub(crate) fn expiry(
        &self,
        id: ToastId,
        timeout: Option<Duration>,
        cx: &mut Context<'_, Self>,
    ) -> Expiry {
        let Some(delay) = timeout else {
            return (None, None, None);
        };
        if !self.holds.is_empty() {
            return (None, None, Some(delay));
        }
        let due = cx.background_executor().now() + delay;
        (
            Some(Self::after(delay, id, cx, Self::dismiss)),
            Some(due),
            None,
        )
    }

    pub(crate) fn hold(
        &mut self,
        by: ToastHold,
        cx: &mut Context<'_, Self>,
    ) {
        if self.holds.contains(&by) {
            return;
        }
        self.holds.push(by);
        if self.holds.len() > 1 {
            return;
        }
        let now = cx.background_executor().now();
        self.entries
            .iter_mut()
            .filter(|entry| !entry.leaving)
            .for_each(|entry| pause(entry, now));
    }

    /// Holds the expiry while `pointer` rests on the column, and releases it when it leaves.
    pub fn follow_pointer(
        &mut self,
        pointer: Pose,
        cx: &mut Context<'_, Self>,
    ) {
        match pointer {
            Pose::On => self.hold(ToastHold::Pointer, cx),
            Pose::Off => self.release(ToastHold::Pointer, cx),
        }
    }

    /// Holds the expiry while `window` does not hold the keyboard, as develop's toasts wait for
    /// someone to look.
    pub fn follow_window(
        &mut self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        if WindowActivity::of(window, cx) {
            return self.release(ToastHold::Window, cx);
        }
        self.hold(ToastHold::Window, cx);
    }

    pub(crate) fn release(
        &mut self,
        by: ToastHold,
        cx: &mut Context<'_, Self>,
    ) {
        let before = self.holds.len();
        self.holds.retain(|hold| *hold != by);
        if self.holds.len() == before || !self.holds.is_empty() {
            return;
        }
        let waiting: Vec<(ToastId, Duration)> = self
            .entries
            .iter_mut()
            .filter(|entry| !entry.leaving)
            .filter_map(|entry| entry.left.take().map(|left| (entry.id, left)))
            .collect();
        for (id, left) in waiting {
            let expiry = self.expiry(id, Some(left), cx);
            self.set_expiry(id, expiry);
        }
    }

    fn set_expiry(
        &mut self,
        id: ToastId,
        expiry: Expiry,
    ) {
        let Some(entry) = self.entries.iter_mut().find(|entry| entry.id == id) else {
            return;
        };
        (entry.timer, entry.due, entry.left) = expiry;
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

fn pause(
    entry: &mut Entry,
    now: Instant,
) {
    let Some(due) = entry.due.take() else {
        return;
    };
    entry.left = Some(due.saturating_duration_since(now));
    entry.timer = None;
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
    use crate::display::toast::toast_queue::timer::toast_hold::ToastHold;
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

    #[gpui::test]
    fn a_held_queue_keeps_its_toasts_until_nothing_holds_and_then_resumes_the_time_they_had_left(
        cx: &mut TestAppContext
    ) {
        let queue = queue(cx);
        push(&queue, ToastLifetime::Auto, cx);
        advance(cx, Duration::from_secs(3));
        queue.update(cx, |queue, cx| queue.hold(ToastHold::Pointer, cx));
        queue.update(cx, |queue, cx| queue.hold(ToastHold::Window, cx));
        push(&queue, ToastLifetime::Auto, cx);
        advance(cx, Duration::from_secs(60));
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 2, "held");
        queue.update(cx, |queue, cx| queue.release(ToastHold::Pointer, cx));
        advance(cx, Duration::from_secs(60));
        assert_eq!(
            queue.read_with(cx, |queue, _| queue.len()),
            2,
            "the inactive window still holds after the pointer leaves"
        );
        queue.update(cx, |queue, cx| queue.release(ToastHold::Window, cx));
        advance(cx, Duration::from_secs(2) + timing::SLOW);
        assert_eq!(
            queue.read_with(cx, |queue, _| queue.len()),
            1,
            "the first had 2 s left"
        );
        advance(cx, Duration::from_secs(3));
        assert_eq!(
            queue.read_with(cx, |queue, _| queue.len()),
            0,
            "the second had all 5 s"
        );
    }

    #[gpui::test]
    fn toasts_keep_expiring_while_a_child_window_holds_the_keyboard(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let window = cx.add_empty_window();
        window.deactivate_window();
        window.update(|window, cx| {
            let id = window.window_handle().window_id();
            crate::WindowActivity::gained(id, cx);
            queue.update(cx, |queue, cx| queue.follow_window(window, cx));
        });
        push(&queue, ToastLifetime::Auto, cx);
        advance(cx, timing::TOAST_TIMEOUT + timing::SLOW);
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 0);
    }
}
