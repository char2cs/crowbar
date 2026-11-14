use serde::{Deserialize, Serialize};

use crate::command_id::CommandId;
use crate::key_chord::KeyChord;

/// One active keybinding.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct KeyBinding {
    keys: KeyChord,
    command: CommandId,
}

impl KeyBinding {
    /// The binding of `keys` to `command`.
    #[must_use]
    pub fn new(keys: KeyChord, command: CommandId) -> Self {
        Self { keys, command }
    }

    /// Key chord in the shell's notation, for example `mod+k`.
    #[must_use]
    pub fn keys(&self) -> &KeyChord {
        &self.keys
    }

    /// The command it runs.
    #[must_use]
    pub fn command(&self) -> &CommandId {
        &self.command
    }
}

#[cfg(test)]
#[path = "key_binding_tests.rs"]
mod tests;
