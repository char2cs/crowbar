use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::pane_instance::PaneInstance;

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
    pub(crate) fn holding(instance: &PaneInstance) -> Self {
        if !instance.type_id.is_chat() {
            return Self::single(instance.pane_id.clone());
        }
        Self {
            chat: Some(instance.pane_id.clone()),
            tabs: Vec::new(),
            active: None,
            pinned: 0,
            editor_open: false,
            chat_selected: true,
        }
    }

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
}

#[cfg(test)]
mod tests {
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn instance(
        name: &str,
        kind: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new(kind).unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    #[test]
    fn a_group_shows_its_chat_until_an_editor_tab_is_selected() {
        let chat = instance("c", PaneTypeId::CHAT);
        let mut group = PaneGroup::holding(&chat);
        assert_eq!(group.shown(), Some(&chat.pane_id), "a lone chat");
        group.tabs = vec![PaneId::new("t").unwrap()];
        group.active = group.tabs.first().cloned();
        assert_eq!(group.shown(), Some(&chat.pane_id), "chat still selected");
        group.chat_selected = false;
        assert_eq!(group.shown(), group.active_tab(), "the tab once selected");
    }

    #[test]
    fn a_pane_that_is_not_a_chat_starts_a_chatless_group_as_its_tab() {
        let terminal = instance("t", "core.terminal");
        let group = PaneGroup::holding(&terminal);
        assert_eq!(group.chat(), None, "no chat");
        assert_eq!(
            group.tabs(),
            std::slice::from_ref(&terminal.pane_id),
            "the tab"
        );
        assert!(group.editor_open(), "its editor shows");
    }
}
