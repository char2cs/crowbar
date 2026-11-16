//! The list of live toasts: expiry, dismissal, pinning, swipe and placement.

use std::time::Duration;

use gpui::{Context, EventEmitter, Pixels, Point, SharedString, Task};

use crate::opaque_debug::opaque_debug;
use crate::timing;
use crate::toast_content::ToastContent;
use crate::toast_entry::ToastEntry;
use crate::toast_id::ToastId;
use crate::toast_lifetime::ToastLifetime;
use crate::toast_placement::ToastPlacement;
use crate::toast_queue_event::ToastQueueEvent;
use crate::toast_swipe::ToastSwipe;

const MAX_TOASTS: usize = 8;

/// A queue entity that owns the toasts of one column; render it inside a relatively positioned container.
///
/// Newest toasts come first. Automatic toasts expire after five seconds, pinned ones stay until
/// [`ToastQueue::dismiss`], the close button or a swipe removes them.
pub struct ToastQueue {
    pub(crate) entries: Vec<ToastEntry>,
    pub(crate) placement: ToastPlacement,
    pub(crate) swipe: Option<ToastSwipe>,
    next: u64,
}

impl EventEmitter<ToastQueueEvent> for ToastQueue {}

impl ToastQueue {
    /// An empty queue shown at `placement`.
    pub fn new(placement: ToastPlacement) -> Self {
        Self {
            entries: Vec::new(),
            placement,
            swipe: None,
            next: 0,
        }
    }

    /// Adds a toast on top of the others.
    pub fn push(
        &mut self,
        content: ToastContent,
        lifetime: ToastLifetime,
        cx: &mut Context<'_, Self>,
    ) -> ToastId {
        self.insert(None, (content, lifetime), cx)
    }

    /// Shows a toast under `key`: a live toast with the same key is rewritten in place (replaying its
    /// shake or pulse), otherwise a new one is added.
    pub fn show(
        &mut self,
        key: impl Into<SharedString>,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) -> ToastId {
        let key = key.into();
        let held = self
            .entries
            .iter()
            .find(|entry| entry.key.as_ref() == Some(&key) && !entry.leaving)
            .map(|entry| entry.id);
        let Some(id) = held else {
            return self.insert(Some(key), (content, lifetime), cx);
        };
        self.restyle(id, (content, lifetime), cx);
        id
    }

    /// Dismisses the live toast shown under `key`, if any.
    pub fn clear(
        &mut self,
        key: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let held = self
            .entries
            .iter()
            .find(|entry| {
                entry.key.as_ref().is_some_and(|held| held.as_ref() == key) && !entry.leaving
            })
            .map(|entry| entry.id);
        if let Some(id) = held {
            self.dismiss(id, cx);
        }
    }

    fn insert(
        &mut self,
        key: Option<SharedString>,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) -> ToastId {
        let ToastContent { kind, message } = content;
        let id = ToastId(self.next);
        self.next += 1;
        let timer = Self::expiry(id, lifetime.timeout(kind), cx);
        self.entries.insert(
            0,
            ToastEntry {
                id,
                key,
                kind,
                message,
                lifetime,
                updates: 0,
                leaving: false,
                timer,
            },
        );
        self.evict_overflow(cx);
        cx.notify();
        id
    }

    fn evict_overflow(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        while self.entries.len() > MAX_TOASTS {
            let oldest_unpinned = self.entries.iter().rposition(|entry| !entry.is_pinned());
            let index = oldest_unpinned.unwrap_or(self.entries.len() - 1);
            let evicted = self.entries.remove(index);
            cx.emit(ToastQueueEvent::Removed(evicted.id));
        }
    }

    fn restyle(
        &mut self,
        id: ToastId,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) {
        let ToastContent { kind, message } = content;
        let Some(entry) = self.entries.iter_mut().find(|entry| entry.id == id) else {
            return;
        };
        entry.kind = kind;
        entry.message = message;
        entry.lifetime = lifetime;
        entry.updates += 1;
        entry.timer = Self::expiry(id, lifetime.timeout(kind), cx);
        cx.notify();
    }

    /// Rewrites a toast in place, replays its error shake or success pulse and restarts its expiry.
    pub fn replace(
        &mut self,
        id: ToastId,
        content: ToastContent,
        cx: &mut Context<'_, Self>,
    ) {
        let live = self
            .entries
            .iter()
            .find(|entry| entry.id == id && !entry.leaving)
            .map(|entry| entry.lifetime);
        let Some(lifetime) = live else {
            return;
        };
        self.restyle(id, (content, lifetime), cx);
    }

    /// Fades a toast out and removes it.
    pub fn dismiss(
        &mut self,
        id: ToastId,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(entry) = self.entries.iter_mut().find(|entry| entry.id == id) else {
            return;
        };
        if entry.leaving {
            return;
        }
        entry.leaving = true;
        entry.timer = Some(Self::after(timing::SLOW, id, cx, Self::remove));
        cx.notify();
    }

    /// Moves the column, for example when the sidebar collapses.
    pub fn set_placement(
        &mut self,
        placement: ToastPlacement,
        cx: &mut Context<'_, Self>,
    ) {
        self.placement = placement;
        cx.notify();
    }

    /// Where the column sits now.
    pub fn placement(&self) -> ToastPlacement {
        self.placement
    }

    /// How many toasts the queue holds, leaving ones included.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether the queue holds no toast.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    pub(crate) fn begin_swipe(
        &mut self,
        toast: ToastId,
        origin: Point<Pixels>,
    ) {
        self.swipe = Some(ToastSwipe { toast, origin });
    }

    pub(crate) fn drag_swipe(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(swipe) = self.swipe else {
            return;
        };
        if !swipe.dismisses(position, self.placement.side()) {
            return;
        }
        self.swipe = None;
        self.dismiss(swipe.toast, cx);
    }

    fn expiry(
        id: ToastId,
        timeout: Option<Duration>,
        cx: &mut Context<'_, Self>,
    ) -> Option<Task<()>> {
        timeout.map(|delay| Self::after(delay, id, cx, Self::dismiss))
    }

    fn after(
        delay: Duration,
        id: ToastId,
        cx: &mut Context<'_, Self>,
        then: fn(&mut Self, ToastId, &mut Context<'_, Self>),
    ) -> Task<()> {
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(delay).await;
            this.update(cx, |queue, cx| then(queue, id, cx)).ok();
        })
    }

    fn remove(
        &mut self,
        id: ToastId,
        cx: &mut Context<'_, Self>,
    ) {
        self.entries.retain(|entry| entry.id != id);
        cx.emit(ToastQueueEvent::Removed(id));
        cx.notify();
    }
}

opaque_debug!(ToastQueue);

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{AppContext as _, Entity, TestAppContext};

    use super::*;
    use crate::toast_kind::ToastKind;

    fn queue(cx: &mut TestAppContext) -> Entity<ToastQueue> {
        cx.new(|_| ToastQueue::new(ToastPlacement::default()))
    }

    fn push(
        queue: &Entity<ToastQueue>,
        lifetime: ToastLifetime,
        cx: &mut TestAppContext,
    ) -> ToastId {
        queue.update(cx, |queue, cx| {
            queue.push(ToastContent::new(ToastKind::Info, "Saved"), lifetime, cx)
        })
    }

    fn show_daemon(
        queue: &Entity<ToastQueue>,
        (text, lifetime): (&'static str, ToastLifetime),
        cx: &mut TestAppContext,
    ) -> ToastId {
        queue.update(cx, |queue, cx| {
            let content = ToastContent::new(ToastKind::Error, text);
            queue.show("daemon", (content, lifetime), cx)
        })
    }

    fn record_removals(
        queue: &Entity<ToastQueue>,
        sink: &Rc<RefCell<Vec<ToastQueueEvent>>>,
        cx: &mut TestAppContext,
    ) {
        let sink = Rc::clone(sink);
        let record = move |_, event: &ToastQueueEvent, _: &mut gpui::App| {
            sink.borrow_mut().push(*event);
        };
        cx.update(|cx| cx.subscribe(queue, record).detach());
    }

    fn advance(
        cx: &mut TestAppContext,
        by: Duration,
    ) {
        cx.executor().advance_clock(by);
        cx.run_until_parked();
    }

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
    fn pinned_toasts_are_capped_and_the_oldest_goes_first(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let ids: Vec<ToastId> = (0..MAX_TOASTS + 3)
            .map(|_| push(&queue, ToastLifetime::Pinned, cx))
            .collect();
        let kept = queue.read_with(cx, |queue, _| {
            queue.entries.iter().map(|e| e.id).collect::<Vec<_>>()
        });
        assert_eq!(kept.len(), MAX_TOASTS);
        assert_eq!(kept.first(), ids.last());
        assert!(!kept.contains(&ids[0]));
    }

    #[gpui::test]
    fn dismissing_fades_first_then_removes_and_reports_it(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let removed = Rc::new(RefCell::new(Vec::new()));
        record_removals(&queue, &removed, cx);
        let id = push(&queue, ToastLifetime::Pinned, cx);
        queue.update(cx, |queue, cx| queue.dismiss(id, cx));
        let leaving = queue.read_with(cx, |queue, _| {
            queue.entries.first().is_some_and(|e| e.leaving)
        });
        assert!(leaving);
        assert!(removed.borrow().is_empty());
        advance(cx, timing::SLOW);
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 0);
        assert_eq!(*removed.borrow(), [ToastQueueEvent::Removed(id)]);
    }

    #[gpui::test]
    fn replacing_restarts_the_expiry_and_counts_the_update(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let id = push(&queue, ToastLifetime::Auto, cx);
        advance(cx, Duration::from_secs(4));
        queue.update(cx, |queue, cx| {
            queue.replace(id, ToastContent::new(ToastKind::Success, "Done"), cx);
        });
        advance(cx, Duration::from_secs(4));
        let state = queue.read_with(cx, |queue, _| {
            queue
                .entries
                .first()
                .map(|e| (e.updates, e.kind, e.message.to_string()))
        });
        assert_eq!(state, Some((1, ToastKind::Success, "Done".to_owned())));
        advance(cx, Duration::from_secs(2));
        assert_eq!(queue.read_with(cx, |queue, _| queue.len()), 0);
    }

    #[gpui::test]
    fn a_keyed_toast_is_rewritten_in_place_and_clearing_dismisses_it(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let first = show_daemon(&queue, ("down", ToastLifetime::Pinned), cx);
        let second = show_daemon(&queue, ("still down", ToastLifetime::Pinned), cx);
        assert_eq!(first, second);
        let state = queue.read_with(cx, |queue, _| {
            queue
                .entries
                .iter()
                .map(|e| (e.updates, e.message.to_string()))
                .collect::<Vec<_>>()
        });
        assert_eq!(state, [(1, "still down".to_owned())]);
        advance(cx, Duration::from_secs(600));
        assert_eq!(
            queue.read_with(cx, |queue, _| queue.len()),
            1,
            "pinned stays"
        );
        queue.update(cx, |queue, cx| queue.clear("absent", cx));
        queue.update(cx, |queue, cx| queue.clear("daemon", cx));
        advance(cx, timing::SLOW);
        assert!(queue.read_with(cx, |queue, _| queue.is_empty()));
        let third = show_daemon(&queue, ("down again", ToastLifetime::Auto), cx);
        assert_ne!(first, third, "a cleared key starts a new toast");
    }

    #[gpui::test]
    fn the_newest_toast_comes_first_and_unknown_ids_are_ignored(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let first = push(&queue, ToastLifetime::Pinned, cx);
        let second = push(&queue, ToastLifetime::Pinned, cx);
        queue.update(cx, |queue, cx| {
            queue.dismiss(ToastId(99), cx);
            queue.replace(ToastId(99), ToastContent::new(ToastKind::Error, "x"), cx);
        });
        let order = queue.read_with(cx, |queue, _| {
            queue.entries.iter().map(|e| e.id).collect::<Vec<_>>()
        });
        assert_eq!(order, [second, first]);
    }

    #[gpui::test]
    fn a_swipe_past_the_threshold_dismisses_the_toast(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let id = push(&queue, ToastLifetime::Pinned, cx);
        queue.update(cx, |queue, cx| {
            queue.begin_swipe(id, gpui::point(gpui::px(200.0), gpui::px(10.0)));
            queue.drag_swipe(gpui::point(gpui::px(180.0), gpui::px(10.0)), cx);
        });
        let leaving = queue.read_with(cx, |queue, _| {
            queue.entries.first().is_some_and(|e| e.leaving)
        });
        assert!(!leaving, "20px is below the threshold");
        queue.update(cx, |queue, cx| {
            queue.drag_swipe(gpui::point(gpui::px(150.0), gpui::px(10.0)), cx);
        });
        let leaving = queue.read_with(cx, |queue, _| {
            queue.entries.first().is_some_and(|e| e.leaving)
        });
        assert!(leaving);
    }
}
