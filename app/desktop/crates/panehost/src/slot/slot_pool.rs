use std::collections::BTreeMap;

use webview::{WebViewHandle, WebViewKind};

use crate::slot::Slot;
use crate::slot::slot_id::SlotId;

#[derive(Debug)]
pub(crate) struct SlotPool {
    slots: BTreeMap<SlotId, Slot>,
    max_idle: usize,
}

impl SlotPool {
    pub(crate) fn new(max_idle: usize) -> Self {
        Self {
            slots: BTreeMap::new(),
            max_idle,
        }
    }

    pub(crate) fn find_free(
        &self,
        kind: &WebViewKind,
    ) -> Option<SlotId> {
        self.slots
            .values()
            .find(|slot| &slot.kind == kind && slot.owner.current().is_none())
            .map(|slot| slot.id)
    }

    pub(crate) fn insert(
        &mut self,
        slot: Slot,
    ) -> SlotId {
        let id = slot.id;
        self.slots.insert(id, slot);
        id
    }

    pub(crate) fn get(
        &self,
        id: SlotId,
    ) -> Option<&Slot> {
        self.slots.get(&id)
    }

    pub(crate) fn views(&self) -> impl Iterator<Item = (&WebViewKind, &WebViewHandle)> {
        self.slots.values().map(|slot| (&slot.kind, &slot.handle))
    }

    pub(crate) fn len(&self) -> usize {
        self.slots.len()
    }

    pub(crate) fn release(
        &mut self,
        id: SlotId,
    ) {
        let Some(slot) = self.slots.get(&id) else {
            return;
        };
        slot.owner.clear();
        if self.free_count() > self.max_idle || slot.kind != WebViewKind::Core {
            self.slots.remove(&id);
        }
    }

    pub(crate) fn discard(
        &mut self,
        id: SlotId,
    ) {
        self.slots.remove(&id);
    }

    fn free_count(&self) -> usize {
        self.slots
            .values()
            .filter(|slot| slot.owner.current().is_none())
            .count()
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use webview::{MockPlatform, ParentWindow, WebViewHost};

    use super::*;
    use crate::slot::slot_owner::SlotOwner;
    use crate::test_support::blank_config;

    fn slot(
        host: &WebViewHost,
        id: u64,
        kind: WebViewKind,
    ) -> Slot {
        let config = blank_config(kind.clone());
        let handle = host.create_in(ParentWindow::X11(1), config).unwrap();
        Slot {
            id: SlotId(id),
            kind,
            handle,
            owner: SlotOwner::default(),
        }
    }

    fn host() -> WebViewHost {
        WebViewHost::with_platform(Rc::new(MockPlatform::default()))
    }

    #[test]
    fn a_free_slot_of_the_right_kind_is_found() {
        let host = host();
        let mut pool = SlotPool::new(2);
        let ext = WebViewKind::Extension(protocol::ExtensionId::new("git").unwrap());
        let core = pool.insert(slot(&host, 1, WebViewKind::Core));
        pool.insert(slot(&host, 2, ext.clone()));
        assert_eq!(pool.find_free(&WebViewKind::Core), Some(core), "core");
        assert!(pool.find_free(&ext).is_some(), "extension");
        pool.get(core)
            .unwrap()
            .owner
            .assign(protocol::PaneId::new("p").unwrap());
        assert_eq!(pool.find_free(&WebViewKind::Core), None, "taken");
    }

    type Case = (&'static str, WebViewKind, usize, usize, usize);

    fn git() -> WebViewKind {
        WebViewKind::Extension(protocol::ExtensionId::new("git").unwrap())
    }

    fn cases() -> Vec<Case> {
        vec![
            (
                "core slots are kept up to the idle limit",
                WebViewKind::Core,
                1,
                3,
                1,
            ),
            (
                "core slots under the limit are all kept",
                WebViewKind::Core,
                4,
                3,
                3,
            ),
            ("extension slots are never reused", git(), 4, 1, 0),
        ]
    }

    fn kept_after_releasing(
        kind: &WebViewKind,
        limit: usize,
        released: u64,
    ) -> usize {
        let host = host();
        let mut pool = SlotPool::new(limit);
        for n in 0..released {
            let id = pool.insert(slot(&host, n, kind.clone()));
            let owned = pool.get(id).unwrap();
            owned.owner.assign(protocol::PaneId::new("p").unwrap());
            pool.release(id);
        }
        pool.len()
    }

    #[test]
    fn released_slots_are_kept_or_destroyed_by_kind() {
        let wrong: Vec<&str> = cases()
            .into_iter()
            .filter(|(_, kind, limit, released, kept)| {
                kept_after_releasing(kind, *limit, *released as u64) != *kept
            })
            .map(|(name, ..)| name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }
}
