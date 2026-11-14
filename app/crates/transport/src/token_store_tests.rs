use std::sync::Arc;

use super::*;
use crate::memory_token_store::MemoryTokenStore;

#[test]
fn a_store_is_usable_as_a_shared_trait_object_and_forgets_on_delete() {
    let store: Arc<dyn TokenStore> = Arc::new(MemoryTokenStore::default());
    assert_eq!(store.get("h").unwrap(), None);
    store.set("h", "entry").unwrap();
    assert_eq!(store.get("h").unwrap().as_deref(), Some("entry"));
    store.delete("h").unwrap();
    assert_eq!(store.get("h").unwrap(), None);
}
