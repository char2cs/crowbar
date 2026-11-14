//! One rebindable command.

use crate::chord::Chord;

/// A command with its default chord; users rebind it through the keybindings page.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Shortcut {
    pub(crate) command: String,
    pub(crate) label: String,
    pub(crate) default: Option<Chord>,
}

impl Shortcut {
    /// A shortcut for `command`; an empty or unparsable `default` leaves it unbound.
    pub fn new(command: impl Into<String>, label: impl Into<String>, default: &str) -> Self {
        Self {
            command: command.into(),
            label: label.into(),
            default: Chord::parse(default),
        }
    }

    /// The command id.
    pub fn command(&self) -> &str {
        &self.command
    }
}

#[cfg(test)]
#[path = "shortcut_tests.rs"]
mod tests;
