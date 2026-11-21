//! What the Files and Changes panels were last told about the focused workspace.

mod asking;
mod file_entry;
mod ignore_rules;
mod rule_set;

pub(crate) use file_entry::FileEntry;

use std::collections::{HashMap, HashSet};

use sidebar::{ChangedFile, Changes, GitStatus};

use crate::live::held::OpenThreads;
use crate::live::held::panel_state::asking::Asking;
use crate::live::held::panel_state::ignore_rules::IgnoreRules;

#[derive(Debug, Clone, Default)]
pub(crate) struct PanelState {
    pub(crate) chat: Option<String>,
    pub(crate) asking: Asking,
    pub(crate) listings: HashMap<String, Vec<FileEntry>>,
    pub(crate) expanded: HashSet<String>,
    pub(crate) statuses: HashMap<String, GitStatus>,
    pub(crate) ignores: IgnoreRules,
    pub(crate) changes: Changes,
    pub(crate) staged: HashSet<String>,
    pub(crate) open_threads: OpenThreads,
    pub(crate) committing: bool,
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

    pub(crate) fn ask(&mut self) -> Option<String> {
        if self.asking == Asking::Sent {
            return None;
        }
        let chat = self.chat.clone()?;
        self.asking = Asking::Sent;
        Some(chat)
    }

    pub(crate) fn listing_root(&self) -> bool {
        self.asking == Asking::Sent && !self.listings.contains_key("")
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

    pub(crate) fn learn_ignore(
        &mut self,
        chat: &str,
        directory: &str,
        text: &str,
    ) {
        if self.chat.as_deref() == Some(chat) {
            self.ignores.learn(directory, text);
        }
    }

    pub(crate) fn change(
        &mut self,
        chat: &str,
        (working, listed): (Changes, Vec<ChangedFile>),
    ) {
        if self.chat.as_deref() != Some(chat) {
            return;
        }
        self.statuses = working
            .files
            .iter()
            .map(|file| (file.path.clone(), file.status))
            .collect();
        self.changes = Changes {
            files: listed,
            ..working
        };
    }

    pub(crate) fn fail(
        &mut self,
        chat: &str,
        error: String,
    ) {
        if self.chat.as_deref() != Some(chat) {
            return;
        }
        self.statuses.clear();
        self.staged.clear();
        self.changes = Changes {
            error: Some(error),
            ..Changes::default()
        };
    }

    pub(crate) fn staged_and_threads(
        &mut self,
        chat: &str,
        (staged, open_threads): (HashSet<String>, OpenThreads),
    ) {
        if self.chat.as_deref() == Some(chat) {
            self.staged = staged;
            self.open_threads = open_threads;
        }
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
