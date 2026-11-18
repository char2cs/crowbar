//! One rebindable command.

use chord::Chord;

/// A command with its default chord; users rebind it through the keybindings page.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Shortcut {
    pub(crate) command: String,
    pub(crate) label: String,
    pub(crate) default: Option<Chord>,
    pub(crate) presets: Vec<(String, Chord)>,
}

impl Shortcut {
    /// The name of the command shown to the user.
    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

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
            presets: Vec::new(),
        }
    }

    /// The chord the preset `preset` binds instead of the default; an unparsable `chord` is ignored.
    #[must_use]
    pub fn in_preset(
        mut self,
        preset: &str,
        chord: &str,
    ) -> Self {
        self.presets
            .extend(Chord::parse(chord).map(|chord| (preset.to_owned(), chord)));
        self
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
