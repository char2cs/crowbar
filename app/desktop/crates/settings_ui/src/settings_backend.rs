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
    fn get(&self, key: &Key) -> Option<Value>;

    /// Stores `value` under `key`.
    ///
    /// # Errors
    /// [`Error::Rejected`] when the backend refuses the value, [`Error::Unavailable`] when it cannot be reached.
    fn set(&self, key: &Key, value: Value) -> Result<(), Error>;

    /// Runs the backend action `id`, such as `browser.clearData`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the action cannot run.
    fn invoke(&self, id: &str) -> Result<(), Error>;

    /// Reports every change to `listener` until the subscription is dropped.
    fn subscribe(&self, listener: Listener) -> Subscription;
}

#[cfg(test)]
#[path = "settings_backend_tests.rs"]
mod tests;
