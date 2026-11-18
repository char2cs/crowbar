//! A titled list of shortcuts that share one overrides key.

use crate::keys::shortcut::Shortcut;
use crate::values::Key;

/// A category of shortcuts; custom chords of every group live in the map stored under `overrides`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ShortcutGroup {
    /// The category title.
    pub title: String,
    pub(crate) overrides: Key,
    /// The shortcuts of the group, in order.
    pub shortcuts: Vec<Shortcut>,
    pub(crate) preset: Option<Key>,
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
            preset: None,
        }
    }

    /// Takes the active preset from the setting `key`.
    #[must_use]
    pub fn preset_from(
        mut self,
        key: impl Into<Key>,
    ) -> Self {
        self.preset = Some(key.into());
        self
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
