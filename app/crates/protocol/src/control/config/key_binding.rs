use serde::{Deserialize, Serialize};

use crate::control::config::key_chord::KeyChord;
use crate::control::config::precedence::Precedence;
use crate::identity::CommandId;

/// One active keybinding.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct KeyBinding {
    keys: KeyChord,
    command: CommandId,
    #[serde(default)]
    precedence: Precedence,
}

impl KeyBinding {
    /// The binding of `keys` to `command`.
    #[must_use]
    pub fn new(
        keys: KeyChord,
        command: CommandId,
    ) -> Self {
        Self {
            keys,
            command,
            precedence: Precedence::Shell,
        }
    }

    /// The same binding with `precedence`.
    #[must_use]
    pub fn with_precedence(
        self,
        precedence: Precedence,
    ) -> Self {
        Self { precedence, ..self }
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

    /// Who has the first say when a page also handles the chord.
    #[must_use]
    pub fn precedence(&self) -> Precedence {
        self.precedence
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trips_and_validates_both_parts() {
        let binding = KeyBinding::new(
            KeyChord::new("mod+k").unwrap(),
            CommandId::new("core.palette").unwrap(),
        );
        let text = serde_json::to_string(&binding).unwrap();
        assert_eq!(
            text, r#"{"keys":"mod+k","command":"core.palette","precedence":"shell"}"#,
            "wire shape"
        );
        let yielding = binding.clone().with_precedence(Precedence::Page);
        assert!(
            serde_json::to_string(&yielding)
                .unwrap()
                .ends_with(r#""precedence":"page"}"#),
            "a page-first binding says so"
        );
        assert_eq!(
            serde_json::from_str::<KeyBinding>(r#"{"keys":"mod+k","command":"core.palette"}"#)
                .unwrap(),
            binding,
            "an older frame without precedence is shell-first"
        );
        assert_eq!(
            serde_json::from_str::<KeyBinding>(&text).unwrap(),
            binding,
            "round trip"
        );
        assert_eq!(binding.keys().as_str(), "mod+k", "keys");
        assert_eq!(binding.command().as_str(), "core.palette", "command");
        assert!(
            serde_json::from_str::<KeyBinding>(r#"{"keys":"mod k","command":"x"}"#).is_err(),
            "bad chord"
        );
        assert!(
            serde_json::from_str::<KeyBinding>(r#"{"keys":"k","command":"a b"}"#).is_err(),
            "bad command"
        );
    }
}
