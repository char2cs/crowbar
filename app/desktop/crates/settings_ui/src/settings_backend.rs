//! The modal's access to setting values.

use std::rc::Rc;

use crate::change::Change;
use crate::error::Error;
use crate::key::Key;
use crate::snapshot::Snapshot;
use crate::subscription::Subscription;
use crate::value::Value;

/// Callback receiving every [`Change`] a backend reports.
pub type Listener = Rc<dyn Fn(&Change)>;

/// Where settings live; the modal reads once, writes on every edit and follows changes.
///
/// The view calls this on the UI thread only: an implementation that talks to the daemon must
/// return quickly and deliver [`Change`]s from its own task.
pub trait SettingsBackend {
    /// Everything the modal needs before its first paint.
    fn snapshot(&self) -> Snapshot;

    /// The stored value of `key`; `None` while it is unset.
    fn get(
        &self,
        key: &Key,
    ) -> Option<Value>;

    /// Stores `value` under `key`.
    ///
    /// # Errors
    /// [`Error::Rejected`] when the backend refuses the value, [`Error::Unavailable`] when it cannot be reached.
    fn set(
        &self,
        key: &Key,
        value: Value,
    ) -> Result<(), Error>;

    /// Runs the backend action `id`, such as `browser.clearData`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the action cannot run.
    fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error>;

    /// Reports every change to `listener` until the subscription is dropped.
    fn subscribe(
        &self,
        listener: Listener,
    ) -> Subscription;
}

#[cfg(test)]
mod tests {
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
}
