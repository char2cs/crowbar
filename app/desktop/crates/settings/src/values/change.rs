//! A notification from the backend.

use crate::values::key::Key;
use crate::values::value::Value;

/// Something that changed behind the modal's back.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Change {
    /// A setting has a new value (another window, a file edit, the daemon).
    Setting(Key, Value),
    /// The record list `source` has new content.
    Records(String),
}
