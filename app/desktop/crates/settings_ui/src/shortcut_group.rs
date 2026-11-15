//! A titled list of shortcuts that share one overrides key.

use crate::key::Key;
use crate::shortcut::Shortcut;

/// A category of shortcuts; custom chords of every group live in the map stored under `overrides`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ShortcutGroup {
    pub(crate) title: String,
    pub(crate) overrides: Key,
    pub(crate) shortcuts: Vec<Shortcut>,
}

impl ShortcutGroup {
    /// An empty group titled `title`.
    pub fn new(
        title: impl Into<String>,
        overrides: impl Into<Key>,
    ) -> Self {
        Self {
            title: title.into(),
            overrides: overrides.into(),
            shortcuts: Vec::new(),
        }
    }

    /// Appends a shortcut.
    #[must_use]
    pub fn shortcut(
        mut self,
        shortcut: Shortcut,
    ) -> Self {
        self.shortcuts.push(shortcut);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shortcuts_keep_their_order() {
        let group = ShortcutGroup::new("Tabs", "overrides")
            .shortcut(Shortcut::new("a", "A", "mod+a"))
            .shortcut(Shortcut::new("b", "B", ""));
        let commands: Vec<&str> = group.shortcuts.iter().map(Shortcut::command).collect();
        assert_eq!(commands, ["a", "b"]);
        assert_eq!(group.overrides.as_str(), "overrides");
    }
}
