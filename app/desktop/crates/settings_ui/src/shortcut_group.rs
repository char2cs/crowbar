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
    pub fn new(title: impl Into<String>, overrides: impl Into<Key>) -> Self {
        Self {
            title: title.into(),
            overrides: overrides.into(),
            shortcuts: Vec::new(),
        }
    }

    /// Appends a shortcut.
    #[must_use]
    pub fn shortcut(mut self, shortcut: Shortcut) -> Self {
        self.shortcuts.push(shortcut);
        self
    }
}

#[cfg(test)]
#[path = "shortcut_group_tests.rs"]
mod tests;
