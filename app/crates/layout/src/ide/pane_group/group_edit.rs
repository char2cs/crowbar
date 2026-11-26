use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;

impl PaneGroup {
    pub(crate) fn activate(
        &mut self,
        pane: &PaneId,
    ) {
        if self.chat.as_ref() == Some(pane) {
            self.chat_selected = true;
        }
        if self.tabs.contains(pane) {
            self.active = Some(pane.clone());
            self.chat_selected = false;
        }
    }

    pub(crate) fn check_index(
        index: Option<usize>,
        others: usize,
    ) -> Result<usize> {
        let at = index.unwrap_or(others);
        (at <= others).then_some(at).ok_or(Error::IndexOutOfRange {
            index: at,
            len: others,
        })
    }

    pub(crate) fn insert(
        &mut self,
        pane: PaneId,
        index: Option<usize>,
    ) -> Result<()> {
        let at = Self::check_index(index, self.tabs.len())?.max(self.pinned);
        if self.tabs.is_empty() {
            self.editor_open = true;
        }
        self.tabs.insert(at, pane.clone());
        self.active = Some(pane);
        self.chat_selected = false;
        Ok(())
    }

    pub(crate) fn take(
        &mut self,
        pane: &PaneId,
    ) {
        if self.chat.as_ref() == Some(pane) {
            self.chat = None;
            return;
        }
        let Some(position) = self.tabs.iter().position(|member| member == pane) else {
            return;
        };
        self.tabs.remove(position);
        if self.preview.as_ref() == Some(pane) {
            self.preview = None;
        }
        if position < self.pinned {
            self.pinned -= 1;
        }
        if self.tabs.is_empty() {
            self.editor_open = false;
        }
        if self.active.as_ref() == Some(pane) {
            self.active = self
                .tabs
                .get(position)
                .or_else(|| self.tabs.last())
                .cloned();
        }
    }

    pub(crate) fn reorder(
        &mut self,
        pane: &PaneId,
        index: Option<usize>,
    ) -> Result<()> {
        let others = self.tabs.len().saturating_sub(1);
        let at = Self::check_index(index, others)?;
        let at = at.clamp(*self.reach(pane).start(), *self.reach(pane).end());
        self.tabs.retain(|member| member != pane);
        self.tabs.insert(at, pane.clone());
        self.active = Some(pane.clone());
        self.chat_selected = false;
        Ok(())
    }

    pub(crate) fn exchange(
        &mut self,
        a: &PaneId,
        b: &PaneId,
    ) {
        let exchanged = |member: &PaneId| match member {
            same if same == a => b.clone(),
            same if same == b => a.clone(),
            other => other.clone(),
        };
        let members = self
            .tabs
            .iter_mut()
            .chain(&mut self.active)
            .chain(&mut self.preview)
            .chain(&mut self.chat);
        for member in members {
            *member = exchanged(member);
        }
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
        let preview_ok = self
            .preview
            .as_ref()
            .is_none_or(|preview| self.tabs.contains(preview));
        sorted.len() == self.tabs.len()
            && active_ok
            && preview_ok
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
        let chat_among_tabs = PaneGroup {
            chat: Some(id("a")),
            ..group(&["a", "b"], "b")
        };
        let mut emptied = PaneGroup::single(id("a"));
        emptied.take(&id("a"));
        let cases = [
            (group(&["a", "b"], "b"), true),
            (group(&["a", "a"], "a"), false),
            (group(&["a", "b"], "z"), false),
            (group(&[], "a"), false),
            (chat_among_tabs, false),
        ];
        for (group, consistent) in cases {
            assert_eq!(group.is_consistent(), consistent, "{group:?}");
        }
        assert!(emptied.is_empty(), "taking the only pane empties it");
        assert_eq!(emptied.active_tab(), None, "nothing shown");
    }
}
