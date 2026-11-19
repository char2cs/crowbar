use protocol::PaneId;
use serde::{Deserialize, Serialize};

/// One place of a tab's split tree, as develop's `PaneGroup`: at most one chat, and the editor
/// tabs shown beside it or over it.
///
/// A group is never empty. Its active editor tab is one of its tabs, and `None` only when it has
/// none. The first [`PaneGroup::pinned_count`] tabs are pinned: they stay in front of the others,
/// and no tab moves across that line except by pinning or unpinning.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneGroup {
    pub(crate) chat: Option<PaneId>,
    pub(crate) tabs: Vec<PaneId>,
    pub(crate) active: Option<PaneId>,
    pub(crate) pinned: usize,
    pub(crate) editor_open: bool,
    pub(crate) chat_selected: bool,
}

impl PaneGroup {
    pub(crate) fn single(pane: PaneId) -> Self {
        Self {
            chat: None,
            tabs: vec![pane.clone()],
            active: Some(pane),
            pinned: 0,
            editor_open: true,
            chat_selected: false,
        }
    }

    /// The group's chat.
    #[must_use]
    pub fn chat(&self) -> Option<&PaneId> {
        self.chat.as_ref()
    }

    /// The editor tabs in tab-bar order.
    #[must_use]
    pub fn tabs(&self) -> &[PaneId] {
        &self.tabs
    }

    /// The editor tab the editor view shows.
    #[must_use]
    pub fn active_tab(&self) -> Option<&PaneId> {
        self.active.as_ref()
    }

    /// Whether the editor view is open beside the chat.
    #[must_use]
    pub fn editor_open(&self) -> bool {
        self.editor_open
    }

    /// Whether the chat, not an editor tab, is selected when only one of them fits.
    #[must_use]
    pub fn chat_selected(&self) -> bool {
        self.chat_selected
    }

    /// The pane shown when the group has room for one only: the chat when it is selected or
    /// there is no editor tab, else the active editor tab.
    #[must_use]
    pub fn shown(&self) -> Option<&PaneId> {
        let chat = self
            .chat
            .as_ref()
            .filter(|_| self.chat_selected || self.active.is_none());
        chat.or(self.active.as_ref())
    }

    /// Whether `pane` is the chat or one of the tabs.
    #[must_use]
    pub fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.chat.as_ref() == Some(pane) || self.tabs.contains(pane)
    }

    pub(crate) fn members(&self) -> Vec<PaneId> {
        self.chat.iter().chain(&self.tabs).cloned().collect()
    }

    pub(crate) fn len(&self) -> usize {
        self.tabs.len()
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.chat.is_none() && self.tabs.is_empty()
    }

    pub(crate) fn is_consistent(&self) -> bool {
        let mut sorted: Vec<&PaneId> = self.tabs.iter().collect();
        sorted.sort();
        sorted.dedup();
        let active_ok = match &self.active {
            Some(active) => self.tabs.contains(active),
            None => self.tabs.is_empty(),
        };
        let chat_apart = self
            .chat
            .as_ref()
            .is_none_or(|chat| !self.tabs.contains(chat));
        sorted.len() == self.tabs.len()
            && active_ok
            && chat_apart
            && !self.is_empty()
            && self.pinned <= self.tabs.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn group(
        names: &[&str],
        active: &str,
    ) -> PaneGroup {
        PaneGroup {
            tabs: names.iter().map(|name| id(name)).collect(),
            active: Some(id(active)),
            ..PaneGroup::single(id(active))
        }
    }

    #[test]
    fn consistency_needs_distinct_members_and_a_member_as_active() {
        assert!(group(&["a", "b"], "b").is_consistent(), "fine");
        assert!(!group(&["a", "a"], "a").is_consistent(), "duplicate");
        assert!(!group(&["a", "b"], "z").is_consistent(), "active outside");
        assert!(!group(&[], "a").is_consistent(), "empty");
        let chat_among_tabs = PaneGroup {
            chat: Some(id("a")),
            ..group(&["a", "b"], "b")
        };
        assert!(!chat_among_tabs.is_consistent(), "chat among the tabs");
    }
}
