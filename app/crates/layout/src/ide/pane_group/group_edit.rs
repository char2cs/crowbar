use crate::ide::pane_group::DEFAULT_CHAT_SHARE;
use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;

impl PaneGroup {
    #[must_use]
    pub fn chat_share(&self) -> f32 {
        self.chat_share.unwrap_or(DEFAULT_CHAT_SHARE)
    }

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

    fn names(group: &PaneGroup) -> Vec<String> {
        group.tabs().iter().map(ToString::to_string).collect()
    }

    #[test]
    fn inserting_puts_the_pane_at_the_index_and_shows_it() {
        let cases = [
            ("end", None, ["a", "b", "x"]),
            ("front", Some(0), ["x", "a", "b"]),
            ("middle", Some(1), ["a", "x", "b"]),
            ("explicit end", Some(2), ["a", "b", "x"]),
        ];
        for (name, index, expected) in cases {
            let mut stack = group(&["a", "b"], "a");
            stack.insert(id("x"), index).unwrap();
            assert_eq!(names(&stack), expected, "{name}");
            assert_eq!(stack.active_tab(), Some(&id("x")), "{name}: shown");
        }
    }

    #[test]
    fn an_index_past_the_end_is_refused_and_changes_nothing() {
        let mut stack = group(&["a", "b"], "a");
        let before = stack.clone();
        assert_eq!(
            stack.insert(id("x"), Some(3)),
            Err(Error::IndexOutOfRange { index: 3, len: 2 }),
            "past the end"
        );
        assert_eq!(stack, before, "unchanged");
    }

    #[test]
    fn taking_the_active_pane_shows_the_one_that_took_its_place() {
        let cases = [
            ("first", "a", "b"),
            ("middle", "b", "c"),
            ("last", "c", "b"),
        ];
        for (name, taken, expected) in cases {
            let mut stack = group(&["a", "b", "c"], taken);
            stack.take(&id(taken));
            assert_eq!(stack.active_tab(), Some(&id(expected)), "{name}");
            assert!(stack.is_consistent(), "{name}: consistent");
        }
    }

    #[test]
    fn taking_a_hidden_pane_keeps_the_active_one() {
        let mut stack = group(&["a", "b", "c"], "c");
        stack.take(&id("a"));
        assert_eq!(stack.active_tab(), Some(&id("c")), "kept");
        assert_eq!(names(&stack), ["b", "c"], "rest");
    }

    #[test]
    fn taking_the_only_pane_leaves_nothing() {
        let mut stack = PaneGroup::single(id("a"));
        stack.take(&id("a"));
        assert!(stack.is_empty(), "gone");
        assert_eq!(stack.active_tab(), None, "nothing shown");
    }

    #[test]
    fn reordering_counts_positions_among_the_other_members() {
        let cases = [
            ("to the front", "c", Some(0), ["c", "a", "b"]),
            ("to the end", "a", None, ["b", "c", "a"]),
            ("between", "a", Some(1), ["b", "a", "c"]),
            ("stays", "b", Some(1), ["a", "b", "c"]),
        ];
        for (name, moved, index, expected) in cases {
            let mut stack = group(&["a", "b", "c"], "a");
            stack.reorder(&id(moved), index).unwrap();
            assert_eq!(names(&stack), expected, "{name}");
            assert_eq!(stack.active_tab(), Some(&id(moved)), "{name}: shown");
        }
        let mut stack = group(&["a", "b", "c"], "a");
        assert_eq!(
            stack.reorder(&id("a"), Some(3)),
            Err(Error::IndexOutOfRange { index: 3, len: 2 }),
            "past the others"
        );
    }

    #[test]
    fn exchanging_swaps_members_and_the_active_mark() {
        let mut stack = group(&["a", "b", "c"], "a");
        stack.exchange(&id("a"), &id("c"));
        assert_eq!(names(&stack), ["c", "b", "a"], "members");
        assert_eq!(
            stack.active_tab(),
            Some(&id("c")),
            "active follows the place"
        );
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
