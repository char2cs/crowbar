use std::collections::BTreeMap;

use protocol::PaneTypeId;

use crate::error::{Error, Result};
use crate::pane_type::PaneType;

/// The known pane types. The shell never matches on a built-in kind: it looks types up here.
#[derive(Debug, Default, Clone)]
pub struct PaneRegistry {
    types: BTreeMap<PaneTypeId, PaneType>,
}

impl PaneRegistry {
    /// Adds a type.
    ///
    /// # Errors
    /// [`Error::DuplicateType`] when the id is taken.
    pub fn register(&mut self, pane_type: PaneType) -> Result<()> {
        if self.types.contains_key(&pane_type.id) {
            return Err(Error::DuplicateType(pane_type.id));
        }
        self.types.insert(pane_type.id.clone(), pane_type);
        Ok(())
    }

    /// Removes a type, for example when an extension is disabled. Instances of it keep their
    /// state and show a placeholder.
    pub fn unregister(&mut self, id: &PaneTypeId) -> Option<PaneType> {
        self.types.remove(id)
    }

    /// Looks up a type.
    #[must_use]
    pub fn get(&self, id: &PaneTypeId) -> Option<&PaneType> {
        self.types.get(id)
    }

    /// All types in id order.
    pub fn iter(&self) -> impl Iterator<Item = &PaneType> {
        self.types.values()
    }
}

#[cfg(test)]
#[path = "pane_registry_tests.rs"]
mod tests;
