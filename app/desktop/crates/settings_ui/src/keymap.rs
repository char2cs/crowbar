//! Effective chords of the shortcuts: defaults overridden by what the user set.

use std::collections::BTreeMap;

use crate::chord::Chord;
use crate::chord_source::ChordSource;
use crate::document::Document;
use crate::page::Page;
use crate::schema::Schema;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Keymap<'a> {
    schema: &'a Schema,
    document: &'a Document,
}

impl<'a> Keymap<'a> {
    pub(crate) fn new(schema: &'a Schema, document: &'a Document) -> Self {
        Self { schema, document }
    }

    pub(crate) fn overrides(&self, group: &ShortcutGroup) -> BTreeMap<String, String> {
        self.document
            .value(&group.overrides)
            .and_then(|value| value.as_map())
            .cloned()
            .unwrap_or_default()
    }

    pub(crate) fn chord(&self, group: &ShortcutGroup, shortcut: &Shortcut) -> Option<Chord> {
        match self.overrides(group).get(&shortcut.command) {
            Some(text) => Chord::parse(text),
            None => shortcut.default.clone(),
        }
    }

    pub(crate) fn source(&self, group: &ShortcutGroup, shortcut: &Shortcut) -> ChordSource {
        if self.overrides(group).contains_key(&shortcut.command) {
            ChordSource::Custom
        } else {
            ChordSource::Default
        }
    }

    pub(crate) fn conflicts(&self, group: &ShortcutGroup, shortcut: &Shortcut) -> Vec<String> {
        let Some(chord) = self.chord(group, shortcut) else {
            return Vec::new();
        };
        self.schema
            .shortcuts()
            .filter(|(_, other)| other.command != shortcut.command)
            .filter(|(other_group, other)| self.chord(other_group, other).as_ref() == Some(&chord))
            .map(|(_, other)| other.label.clone())
            .collect()
    }

    pub(crate) fn has_custom(&self) -> bool {
        self.schema
            .pages()
            .iter()
            .flat_map(Page::shortcut_groups)
            .any(|group| !self.overrides(group).is_empty())
    }
}

#[cfg(test)]
#[path = "keymap_tests.rs"]
mod tests;
