use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::error::{Error, Result};
use crate::remaining::Remaining;

/// The panes that share one place in a tab's split tree: a strip of stacked panes of which
/// exactly one, the active pane, is shown.
///
/// A group is never empty and its active pane is always one of its members.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Group {
    panes: Vec<PaneId>,
    active: PaneId,
}

impl Group {
    pub(crate) fn single(pane: PaneId) -> Self {
        Self {
            panes: vec![pane.clone()],
            active: pane,
        }
    }

    /// The members in strip order.
    #[must_use]
    pub fn panes(&self) -> &[PaneId] {
        &self.panes
    }

    /// The member that is shown.
    #[must_use]
    pub fn active(&self) -> &PaneId {
        &self.active
    }

    /// Whether `pane` is a member.
    #[must_use]
    pub fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.contains(pane)
    }

    pub(crate) fn len(&self) -> usize {
        self.panes.len()
    }

    pub(crate) fn activate(
        &mut self,
        pane: &PaneId,
    ) {
        if self.contains(pane) {
            self.active = pane.clone();
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
        let at = Self::check_index(index, self.panes.len())?;
        self.panes.insert(at, pane.clone());
        self.active = pane;
        Ok(())
    }

    pub(crate) fn take(
        &mut self,
        pane: &PaneId,
    ) -> Remaining {
        let Some(position) = self.panes.iter().position(|member| member == pane) else {
            return Remaining::Active(self.active.clone());
        };
        self.panes.remove(position);
        let heir = self.panes.get(position).or_else(|| self.panes.last());
        let Some(heir) = heir.cloned() else {
            return Remaining::Nothing;
        };
        if &self.active == pane {
            self.active = heir;
        }
        Remaining::Active(self.active.clone())
    }

    pub(crate) fn reorder(
        &mut self,
        pane: &PaneId,
        index: Option<usize>,
    ) -> Result<()> {
        let others = self.panes.len().saturating_sub(1);
        let at = Self::check_index(index, others)?;
        self.panes.retain(|member| member != pane);
        self.panes.insert(at, pane.clone());
        self.active = pane.clone();
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
        for member in self.panes.iter_mut().chain([&mut self.active]) {
            *member = exchanged(member);
        }
    }

    pub(crate) fn is_consistent(&self) -> bool {
        let mut sorted: Vec<&PaneId> = self.panes.iter().collect();
        sorted.sort();
        sorted.dedup();
        sorted.len() == self.panes.len() && self.contains(&self.active)
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
    ) -> Group {
        Group {
            panes: names.iter().map(|name| id(name)).collect(),
            active: id(active),
        }
    }

    fn names(group: &Group) -> Vec<String> {
        group.panes().iter().map(ToString::to_string).collect()
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
            assert_eq!(stack.active(), &id("x"), "{name}: shown");
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
            assert_eq!(
                stack.take(&id(taken)),
                Remaining::Active(id(expected)),
                "{name}"
            );
            assert!(stack.is_consistent(), "{name}: consistent");
        }
    }

    #[test]
    fn taking_a_hidden_pane_keeps_the_active_one() {
        let mut stack = group(&["a", "b", "c"], "c");
        assert_eq!(stack.take(&id("a")), Remaining::Active(id("c")), "kept");
        assert_eq!(names(&stack), ["b", "c"], "rest");
    }

    #[test]
    fn taking_the_only_pane_leaves_nothing() {
        let mut stack = Group::single(id("a"));
        assert_eq!(stack.take(&id("a")), Remaining::Nothing, "gone");
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
            assert_eq!(stack.active(), &id(moved), "{name}: shown");
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
        assert_eq!(stack.active(), &id("c"), "active follows the place");
    }

    #[test]
    fn consistency_needs_distinct_members_and_a_member_as_active() {
        assert!(group(&["a", "b"], "b").is_consistent(), "fine");
        assert!(!group(&["a", "a"], "a").is_consistent(), "duplicate");
        assert!(!group(&["a", "b"], "z").is_consistent(), "active outside");
        assert!(!group(&[], "a").is_consistent(), "empty");
    }
}
