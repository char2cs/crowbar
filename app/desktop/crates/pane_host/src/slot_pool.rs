use std::collections::BTreeMap;

use webview::WebViewKind;

use crate::slot::Slot;
use crate::slot_id::SlotId;

/// The web views the host owns. Views follow visible panes: a pane borrows a free slot of
/// its kind, and a released slot is reused until more than `max_idle` are free.
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

    pub(crate) fn find_free(&self, kind: &WebViewKind) -> Option<SlotId> {
        self.slots
            .values()
            .find(|slot| &slot.kind == kind && slot.owner.current().is_none())
            .map(|slot| slot.id)
    }

    pub(crate) fn insert(&mut self, slot: Slot) -> SlotId {
        let id = slot.id;
        self.slots.insert(id, slot);
        id
    }

    pub(crate) fn get(&self, id: SlotId) -> Option<&Slot> {
        self.slots.get(&id)
    }

    pub(crate) fn len(&self) -> usize {
        self.slots.len()
    }

    pub(crate) fn release(&mut self, id: SlotId) {
        let Some(slot) = self.slots.get(&id) else {
            return;
        };
        slot.owner.clear();
        if self.free_count() > self.max_idle || slot.kind != WebViewKind::Core {
            self.slots.remove(&id);
        }
    }

    fn free_count(&self) -> usize {
        self.slots
            .values()
            .filter(|slot| slot.owner.current().is_none())
            .count()
    }
}

#[cfg(test)]
#[path = "slot_pool_tests.rs"]
mod tests;
