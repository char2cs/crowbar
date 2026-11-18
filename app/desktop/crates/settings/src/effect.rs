//! What the view must do after a state transition.

use crate::key::Key;
use crate::record_command::RecordCommand;
use crate::value::Value;

/// A side effect requested by [`crate::SettingsState`]; the view performs it against the backend.
#[derive(Debug, Clone, PartialEq)]
pub enum Effect {
    /// Store `value` under `key`.
    Write(Key, Value),
    /// Run the backend action with this id.
    Invoke(String),
    /// Apply a command to a record of a list.
    Record {
        /// The list.
        source: String,
        /// The record.
        record: String,
        /// The command.
        command: RecordCommand,
    },
}
