use std::cell::RefCell;
use std::rc::Rc;

use super::*;
use crate::fake_backend::FakeBackend;
use crate::value::Value;

#[test]
fn a_backend_is_usable_behind_the_trait_object() {
    let backend: Rc<dyn SettingsBackend> = Rc::new(FakeBackend::default());
    let seen = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&seen);
    let subscription = backend.subscribe(Rc::new(move |change| {
        sink.borrow_mut().push(change.clone());
    }));
    let key = Key::new("wordWrap");
    backend.set(&key, Value::Bool(true)).unwrap();
    assert_eq!(backend.get(&key), Some(Value::Bool(true)));
    assert_eq!(seen.borrow().len(), 1);
    drop(subscription);
    backend.set(&key, Value::Bool(false)).unwrap();
    assert_eq!(seen.borrow().len(), 1);
}
