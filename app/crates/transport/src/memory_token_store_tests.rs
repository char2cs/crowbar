use super::*;

#[test]
fn the_memory_store_round_trips() {
    let store = MemoryTokenStore::default();
    store.set("h", "e").unwrap();
    assert_eq!(store.get("h").unwrap(), Some("e".into()));
    store.delete("h").unwrap();
    assert_eq!(store.get("h").unwrap(), None);
}
