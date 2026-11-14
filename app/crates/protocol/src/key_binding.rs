use serde::{Deserialize, Serialize};

/// One active keybinding.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct KeyBinding {
    /// Key chord in the shell's notation, for example `mod+k`.
    pub keys: String,
    /// The command it runs.
    pub command: String,
}
