//! The modal's access to setting values.

use crate::values::settings_follow::SettingsFollow;
use crate::values::settings_read::SettingsRead;
use crate::values::settings_write::SettingsWrite;

/// Where settings live; the modal reads once, writes on every edit and follows changes.
///
/// The view calls this on the UI thread only: an implementation that talks to the daemon must
/// return quickly and deliver changes from its own task.
pub trait SettingsBackend: SettingsRead + SettingsWrite + SettingsFollow {}

impl<T: SettingsRead + SettingsWrite + SettingsFollow> SettingsBackend for T {}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use super::*;
    use crate::records::FakeBackend;
    use crate::values::key::Key;
    use crate::values::value::Value;

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
}
