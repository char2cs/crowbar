//! The read side of a settings backend.

use crate::key::Key;
use crate::snapshot::Snapshot;
use crate::value::Value;

/// What the modal reads before it paints and whenever it asks for one value.
pub trait SettingsRead {
    /// Everything the modal needs before its first paint.
    fn snapshot(&self) -> Snapshot;

    /// The stored value of `key`; `None` while it is unset.
    fn get(
        &self,
        key: &Key,
    ) -> Option<Value>;
}
