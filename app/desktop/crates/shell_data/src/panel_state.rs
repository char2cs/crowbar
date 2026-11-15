//! What the Files and Changes panels were last told about the focused workspace.

use std::collections::{HashMap, HashSet};

use sidebar::{Changes, GitStatus};

use crate::file_entry::FileEntry;

#[derive(Debug, Clone, Default)]
pub(crate) struct PanelState {
    pub(crate) chat: Option<String>,
    pub(crate) listings: HashMap<String, Vec<FileEntry>>,
    pub(crate) expanded: HashSet<String>,
    pub(crate) statuses: HashMap<String, GitStatus>,
    pub(crate) changes: Changes,
}

impl PanelState {
    pub(crate) fn follow(
        &mut self,
        chat: Option<String>,
    ) {
        if self.chat == chat {
            return;
        }
        *self = Self {
            chat,
            ..Self::default()
        };
    }

    pub(crate) fn list(
        &mut self,
        chat: &str,
        directory: &str,
        entries: Vec<FileEntry>,
    ) {
        if self.chat.as_deref() == Some(chat) {
            self.listings.insert(directory.to_owned(), entries);
        }
    }

    pub(crate) fn change(
        &mut self,
        chat: &str,
        changes: Changes,
    ) {
        if self.chat.as_deref() != Some(chat) {
            return;
        }
        self.statuses = changes
            .files
            .iter()
            .map(|file| (file.path.clone(), file.status))
            .collect();
        self.changes = changes;
    }

    pub(crate) fn toggle(
        &mut self,
        path: &str,
    ) -> bool {
        if self.expanded.remove(path) {
            return false;
        }
        self.expanded.insert(path.to_owned());
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn following_another_chat_forgets_the_listings_but_the_same_one_keeps_them() {
        let mut panel = PanelState::default();
        panel.follow(Some("a".into()));
        panel.listings.insert(String::new(), Vec::new());
        panel.follow(Some("a".into()));
        assert_eq!(panel.listings.len(), 1);
        panel.follow(Some("b".into()));
        assert!(panel.listings.is_empty());
    }

    #[test]
    fn toggle_alternates() {
        let mut panel = PanelState::default();
        assert!(panel.toggle("src"));
        assert!(!panel.toggle("src"));
    }
}
