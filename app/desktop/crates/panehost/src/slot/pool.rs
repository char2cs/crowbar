use std::collections::BTreeMap;

use crate::slot::Slot;
use crate::slot::id::Id;

#[derive(Debug)]
pub(crate) struct Pool {
    slots: BTreeMap<Id, Slot>,
    max_idle: usize,
}

impl Pool {
    pub(crate) fn new(max_idle: usize) -> Self {
        Self {
            slots: BTreeMap::new(),
            max_idle,
        }
    }

    pub(crate) fn find_free(
        &self,
        kind: &webview::Kind,
    ) -> Option<Id> {
        self.slots
            .values()
            .find(|slot| &slot.kind == kind && slot.owner.current().is_none())
            .map(|slot| slot.id)
    }

    pub(crate) fn insert(
        &mut self,
        slot: Slot,
    ) -> Id {
        let id = slot.id;
        self.slots.insert(id, slot);
        id
    }

    pub(crate) fn get(
        &self,
        id: Id,
    ) -> Option<&Slot> {
        self.slots.get(&id)
    }

    pub(crate) fn views(&self) -> impl Iterator<Item = (&webview::Kind, &webview::Handle)> {
        self.slots.values().map(|slot| (&slot.kind, &slot.handle))
    }

    pub(crate) fn len(&self) -> usize {
        self.slots.len()
    }

    pub(crate) fn release(
        &mut self,
        id: Id,
    ) {
        let Some(slot) = self.slots.get(&id) else {
            return;
        };
        slot.owner.clear();
        if self.free_count() > self.max_idle || slot.kind != webview::Kind::Core {
            self.slots.remove(&id);
        }
    }

    pub(crate) fn clear(&mut self) {
        self.slots.clear();
    }

    pub(crate) fn discard(
        &mut self,
        id: Id,
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

    use webview::{MockPlatform, ParentWindow};

    use super::*;
    use crate::slot::owner::Owner;
    use crate::test_support::blank_config;

    fn owned_slot(
        host: &webview::Host,
        id: Id,
        kind: webview::Kind,
    ) -> Slot {
        let config = blank_config(kind.clone());
        let handle = host.create_in(ParentWindow::X11(1), config).unwrap();
        let owner = Owner::default();
        owner.assign(protocol::PaneId::new("p").unwrap());
        Slot {
            id,
            kind,
            handle,
            owner,
        }
    }

    fn kept_after_releasing(
        kind: &webview::Kind,
        limit: usize,
        released: u64,
    ) -> usize {
        let host = webview::Host::with_platform(Rc::new(MockPlatform::default()));
        let mut pool = Pool::new(limit);
        for n in 0..released {
            let id = pool.insert(owned_slot(&host, Id(n), kind.clone()));
            pool.release(id);
        }
        pool.len()
    }

    #[test]
    fn released_slots_are_kept_or_destroyed_by_kind() {
        let git = webview::Kind::Extension(protocol::ExtensionId::new("git").unwrap());
        let cases = [
            (
                "core slots are kept up to the idle limit",
                webview::Kind::Core,
                1,
                3,
                1,
            ),
            (
                "core slots under the limit are all kept",
                webview::Kind::Core,
                4,
                3,
                3,
            ),
            ("extension slots are never reused", git, 4, 1, 0),
        ];
        for (name, kind, limit, released, kept) in cases {
            assert_eq!(kept_after_releasing(&kind, limit, released), kept, "{name}");
        }
    }
}
