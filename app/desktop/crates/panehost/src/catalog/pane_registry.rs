use std::collections::BTreeMap;

use protocol::PaneTypeId;

use crate::catalog::pane_type::PaneType;
use crate::error::{Error, Result};

#[derive(Debug, Default, Clone)]
pub struct PaneRegistry {
    types: BTreeMap<PaneTypeId, PaneType>,
}

impl PaneRegistry {
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

    pub fn unregister(
        &mut self,
        id: &PaneTypeId,
    ) -> Option<PaneType> {
        self.types.remove(id)
    }

    #[must_use]
    pub fn get(
        &self,
        id: &PaneTypeId,
    ) -> Option<&PaneType> {
        self.types.get(id)
    }

    pub fn iter(&self) -> impl Iterator<Item = &PaneType> {
        self.types.values()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::catalog::multiplicity::Multiplicity;
    use crate::catalog::pane_lifecycle::PaneLifecycle;

    fn pane_type(id: &str) -> PaneType {
        PaneType {
            id: PaneTypeId::new(id).unwrap(),
            title: id.into(),
            entry: "x/index.html".into(),
            kind: crate::catalog::pane_kind::PaneKind::Web,
            lifecycle: PaneLifecycle::Parkable,
            multiplicity: Multiplicity::PerParams,
        }
    }

    #[test]
    fn registers_looks_up_and_unregisters() {
        let mut registry = PaneRegistry::default();
        registry.register(pane_type("core.chat")).unwrap();
        registry.register(pane_type("ext.git.review")).unwrap();
        let id = PaneTypeId::new("core.chat").unwrap();
        assert!(registry.get(&id).is_some(), "registered");
        assert_eq!(registry.iter().count(), 2, "both listed");
        assert!(registry.unregister(&id).is_some(), "removed");
        assert!(registry.get(&id).is_none(), "gone");
    }

    #[test]
    fn duplicate_ids_are_refused() {
        let mut registry = PaneRegistry::default();
        registry.register(pane_type("core.chat")).unwrap();
        assert!(
            matches!(
                registry.register(pane_type("core.chat")),
                Err(Error::DuplicateType(_))
            ),
            "dup"
        );
    }
}
