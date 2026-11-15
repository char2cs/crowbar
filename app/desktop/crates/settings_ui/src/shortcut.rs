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
    pub fn new(
        command: impl Into<String>,
        label: impl Into<String>,
        default: &str,
    ) -> Self {
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
mod tests {
    use super::*;

    #[test]
    fn an_empty_default_is_unbound() {
        assert_eq!(Shortcut::new("tabs.new", "New tab", "").default, None);
        assert_eq!(
            Shortcut::new("tabs.new", "New tab", "Mod+T")
                .default
                .as_ref()
                .map(Chord::as_str),
            Some("mod+t")
        );
        assert_eq!(
            Shortcut::new("tabs.new", "New tab", "").command(),
            "tabs.new"
        );
    }
}
