use std::collections::BTreeMap;

use protocol::PaneTypeId;

use crate::catalog::pane_type::PaneType;
use crate::error::{Error, Result};

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
    pub fn register(
        &mut self,
        pane_type: PaneType,
    ) -> Result<()> {
        if self.types.contains_key(&pane_type.id) {
            return Err(Error::DuplicateType(pane_type.id));
        }
        self.types.insert(pane_type.id.clone(), pane_type);
        Ok(())
    }

    /// Looks up a type.
    #[must_use]
    pub fn get(
        &self,
        id: &PaneTypeId,
    ) -> Option<&PaneType> {
        self.types.get(id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::catalog::PaneLifecycle;
    use crate::test_support::pane_type;

    #[test]
    fn a_type_id_is_registered_once() {
        let mut registry = PaneRegistry::default();
        let chat = || pane_type("core.chat", PaneLifecycle::Parkable);
        registry.register(chat()).unwrap();
        let again = registry.register(chat());
        assert!(matches!(again, Err(Error::DuplicateType(_))), "{again:?}");
    }
}
