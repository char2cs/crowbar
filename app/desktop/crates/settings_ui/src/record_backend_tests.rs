use std::rc::Rc;

use super::*;
use crate::fake_backend::FakeBackend;
use crate::record::Record;

#[test]
fn a_backend_is_usable_behind_the_trait_object() {
    let backend: Rc<dyn RecordBackend> = Rc::new(
        FakeBackend::default().with_records("agents", Records::Ready(vec![Record::new("a", "A")])),
    );
    assert!(matches!(backend.records("agents"), Records::Ready(list) if list.len() == 1));
    assert_eq!(backend.records("elsewhere"), Records::Loading);
    backend
        .apply("agents", "a", RecordCommand::Refresh)
        .unwrap();
}
