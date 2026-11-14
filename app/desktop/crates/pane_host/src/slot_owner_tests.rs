use super::*;

#[test]
fn assignment_is_shared_between_clones() {
    let owner = SlotOwner::default();
    let reader = owner.clone();
    assert_eq!(reader.current(), None, "starts free");
    owner.assign(PaneId::new("p1").unwrap());
    assert_eq!(reader.current(), PaneId::new("p1").ok(), "assigned");
    owner.assign(PaneId::new("p2").unwrap());
    assert_eq!(reader.current(), PaneId::new("p2").ok(), "reassigned");
    owner.clear();
    assert_eq!(reader.current(), None, "cleared");
}
