//! The list of live toasts: expiry, dismissal, pinning, swipe and placement.

use gpui::{Context, EventEmitter, SharedString};

use crate::display::toast::toast_queue::entry::Entry;
use crate::display::toast::toast_queue::surface::swipe::Swipe;
use crate::display::toast::toast_queue::timer::toast_hold::ToastHold;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::timing;

const MAX_TOASTS: usize = 8;

/// A queue entity that owns the toasts of one column; render it inside a relatively positioned container.
///
/// Newest toasts come first. Automatic toasts expire after five seconds, pinned ones stay until
/// [`ToastQueue::dismiss`], the close button or a swipe removes them.
pub struct ToastQueue {
    pub(crate) entries: Vec<Entry>,
    pub(crate) placement: ToastPlacement,
    pub(crate) swipe: Option<Swipe>,
    pub(crate) holds: Vec<ToastHold>,
    next: u64,
}

mod entry;
mod surface;
mod timer;
mod toast_content;
mod toast_id;
mod toast_lifetime;
mod toast_placement;
mod toast_queue_event;
mod view;

pub use toast_content::ToastContent;
pub use toast_id::ToastId;
pub use toast_lifetime::ToastLifetime;
pub use toast_placement::ToastPlacement;
pub use toast_placement::toast_side::ToastSide;
pub use toast_queue_event::ToastQueueEvent;

impl EventEmitter<ToastQueueEvent> for ToastQueue {}

impl ToastQueue {
    /// An empty queue shown at `placement`.
    pub fn new(placement: ToastPlacement) -> Self {
        Self {
            entries: Vec::new(),
            placement,
            swipe: None,
            holds: Vec::new(),
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
        self.clear_matching(|held| held == key, cx);
    }

    /// Dismisses every live toast whose key `matches`, such as all the toasts of a closed pane.
    pub fn clear_matching(
        &mut self,
        matches: impl Fn(&str) -> bool,
        cx: &mut Context<'_, Self>,
    ) {
        let held: Vec<ToastId> = self
            .entries
            .iter()
            .filter(|entry| !entry.leaving)
            .filter(|entry| entry.key.as_ref().is_some_and(|key| matches(key.as_ref())))
            .map(|entry| entry.id)
            .collect();
        for id in held {
            self.dismiss(id, cx);
        }
    }

    fn insert(
        &mut self,
        key: Option<SharedString>,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) -> ToastId {
        let ToastContent {
            kind,
            title,
            message,
        } = content;
        let id = ToastId(self.next);
        self.next += 1;
        let (timer, due, left) = self.expiry(id, lifetime.timeout(kind), cx);
        self.entries.insert(
            0,
            Entry {
                id,
                key,
                kind,
                title,
                message,
                lifetime,
                updates: 0,
                leaving: false,
                timer,
                due,
                left,
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
        let ToastContent {
            kind,
            title,
            message,
        } = content;
        let (timer, due, left) = self.expiry(id, lifetime.timeout(kind), cx);
        let Some(entry) = self.entries.iter_mut().find(|entry| entry.id == id) else {
            return;
        };
        entry.kind = kind;
        entry.title = title;
        entry.message = message;
        entry.lifetime = lifetime;
        entry.updates += 1;
        (entry.timer, entry.due, entry.left) = (timer, due, left);
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

    pub(crate) fn remove(
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

    use std::time::Duration;

    use gpui::{Entity, TestAppContext};

    use super::*;
    use crate::display::toast::test_support::{advance, push, queue};
    use crate::display::toast::toast_kind::ToastKind;

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

    fn show_daemon_as(
        queue: &Entity<ToastQueue>,
        key: &'static str,
        cx: &mut TestAppContext,
    ) {
        queue.update(cx, |queue, cx| {
            let content = ToastContent::new(ToastKind::Info, key);
            queue.show(key, (content, ToastLifetime::Pinned), cx);
        });
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
    fn clearing_by_match_takes_down_every_matching_keyed_toast(cx: &mut TestAppContext) {
        let queue = queue(cx);
        let kept = push(&queue, ToastLifetime::Pinned, cx);
        for key in ["pane:a:1", "pane:a:2", "pane:b:1"] {
            show_daemon_as(&queue, key, cx);
        }
        queue.update(cx, |queue, cx| {
            queue.clear_matching(|key| key.starts_with("pane:a:"), cx);
        });
        advance(cx, timing::SLOW);
        let left = queue.read_with(cx, |queue, _| {
            queue
                .entries
                .iter()
                .map(|e| e.message.to_string())
                .collect::<Vec<_>>()
        });
        assert_eq!(left, ["pane:b:1", "Saved"]);
        assert_eq!(queue.read_with(cx, |queue, _| queue.entries[1].id), kept);
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
}
