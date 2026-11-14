use super::*;
use crate::multiplicity::Multiplicity;
use crate::pane_lifecycle::PaneLifecycle;

fn pane_type(id: &str) -> PaneType {
    PaneType {
        id: PaneTypeId::new(id).unwrap(),
        title: id.into(),
        entry: "x/index.html".into(),
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
