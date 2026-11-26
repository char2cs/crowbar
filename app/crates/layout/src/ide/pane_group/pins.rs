use std::ops::RangeInclusive;

use protocol::PaneId;

use crate::ide::pane_group::PaneGroup;
use crate::ide::pane_group::close_scope::CloseScope;
use crate::ide::pane_group::strip_step::StripStep;

impl PaneGroup {
    pub(crate) fn pinned_count(&self) -> usize {
        self.pinned
    }

    /// Whether `pane` is a pinned tab.
    #[must_use]
    pub fn is_pinned(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.tabs
            .iter()
            .take(self.pinned)
            .any(|member| member == pane)
    }

    /// The tab a keyboard `step` from `from` lands on; `None` at either end of the tab bar, when
    /// already there, or when `from` is not a tab.
    #[must_use]
    pub fn step(
        &self,
        from: &PaneId,
        step: StripStep,
    ) -> Option<&PaneId> {
        let at = self.tabs.iter().position(|member| member == from)?;
        let target = match step {
            StripStep::Previous => at.checked_sub(1)?,
            StripStep::Next => at + 1,
            StripStep::First => 0,
            StripStep::Last => self.tabs.len().saturating_sub(1),
        };
        self.tabs.get(target).filter(|found| *found != from)
    }

    /// The tabs a bulk close started on `anchor` reaches. Pinned tabs are never in it,
    /// and neither is `anchor` itself unless the scope is [`CloseScope::All`].
    #[must_use]
    pub fn closable(
        &self,
        anchor: &PaneId,
        scope: CloseScope,
    ) -> Vec<PaneId> {
        let anchor_at = self.tabs.iter().position(|member| member == anchor);
        self.tabs
            .iter()
            .enumerate()
            .skip(self.pinned)
            .filter(|(at, member)| match scope {
                CloseScope::Others => *member != anchor,
                CloseScope::ToRight => anchor_at.is_some_and(|from| *at > from),
                CloseScope::All => true,
            })
            .map(|(_, member)| member.clone())
            .collect()
    }

    pub(crate) fn reach(
        &self,
        pane: &PaneId,
    ) -> RangeInclusive<usize> {
        let others = self.tabs.len().saturating_sub(1);
        let loose_from = self.pinned - usize::from(self.is_pinned(pane));
        if self.is_pinned(pane) {
            return 0..=loose_from;
        }
        loose_from..=others
    }

    pub(crate) fn pin(
        &mut self,
        pane: &PaneId,
    ) {
        let Some(at) = self.tabs.iter().position(|member| member == pane) else {
            return;
        };
        if at < self.pinned {
            return;
        }
        let moved = self.tabs.remove(at);
        self.tabs.insert(self.pinned, moved);
        self.pinned += 1;
    }

    pub(crate) fn unpin(
        &mut self,
        pane: &PaneId,
    ) {
        if !self.is_pinned(pane) {
            return;
        }
        let Some(at) = self.tabs.iter().position(|member| member == pane) else {
            return;
        };
        let moved = self.tabs.remove(at);
        self.pinned -= 1;
        self.tabs.insert(self.pinned, moved);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn pinned_group(
        names: &[&str],
        pinned: usize,
    ) -> PaneGroup {
        PaneGroup {
            tabs: names.iter().map(|name| id(name)).collect(),
            pinned,
            ..PaneGroup::single(id(names[0]))
        }
    }

    fn names(group: &PaneGroup) -> String {
        let names: Vec<String> = group.tabs().iter().map(ToString::to_string).collect();
        names.join(" ")
    }

    #[test]
    fn a_pane_lands_and_reorders_only_on_its_own_side_of_the_pin_line() {
        let inserted = [
            (Some(0), "a b x c"),
            (Some(1), "a b x c"),
            (Some(2), "a b x c"),
            (None, "a b c x"),
        ];
        for (index, expected) in inserted {
            let mut stack = pinned_group(&["a", "b", "c"], 2);
            stack.insert(id("x"), index).unwrap();
            assert_eq!(names(&stack), expected, "insert at {index:?}");
            assert_eq!(stack.pinned_count(), 2, "{index:?}: pins kept");
        }
        let reordered = [
            ("a", Some(3), "b a c d e"),
            ("b", Some(0), "b a c d e"),
            ("e", Some(0), "a b e c d"),
            ("c", None, "a b d e c"),
        ];
        for (moved, index, expected) in reordered {
            let mut stack = pinned_group(&["a", "b", "c", "d", "e"], 2);
            stack.reorder(&id(moved), index).unwrap();
            assert_eq!(names(&stack), expected, "{moved} to {index:?}");
            assert_eq!(stack.pinned_count(), 2, "{moved}: pins kept");
        }
    }

    #[test]
    fn taking_a_pinned_pane_shrinks_the_pinned_run_and_a_loose_one_leaves_it() {
        let mut stack = pinned_group(&["a", "b", "c"], 2);
        stack.take(&id("c"));
        assert_eq!(stack.pinned_count(), 2, "loose pane taken");
        stack.take(&id("a"));
        assert_eq!(stack.pinned_count(), 1, "pinned pane taken");
        assert!(pinned_group(&["a", "b"], 2).is_consistent(), "all pinned");
        assert!(!pinned_group(&["a", "b"], 3).is_consistent(), "too many");
    }

    #[test]
    fn stepping_walks_the_strip_without_wrapping() {
        let stack = pinned_group(&["a", "b", "c"], 0);
        let cases = [
            ("b", StripStep::Previous, Some("a")),
            ("b", StripStep::Next, Some("c")),
            ("a", StripStep::Previous, None),
            ("c", StripStep::Next, None),
            ("c", StripStep::First, Some("a")),
            ("a", StripStep::Last, Some("c")),
            ("a", StripStep::First, None),
            ("z", StripStep::Next, None),
        ];
        for (from, step, expected) in cases {
            let got = stack.step(&id(from), step).map(PaneId::as_str);
            assert_eq!(got, expected, "{step:?} from {from}");
        }
    }

    #[test]
    fn closable_panes_leave_out_the_pinned_ones_and_the_pane_itself() {
        let stack = pinned_group(&["p", "a", "b", "c"], 1);
        let cases = [
            ("b", CloseScope::Others, "a c"),
            ("a", CloseScope::ToRight, "b c"),
            ("a", CloseScope::All, "a b c"),
            ("c", CloseScope::ToRight, ""),
            ("p", CloseScope::ToRight, "a b c"),
        ];
        for (anchor, scope, expected) in cases {
            let closable = stack.closable(&id(anchor), scope);
            let got: Vec<&str> = closable.iter().map(PaneId::as_str).collect();
            assert_eq!(got.join(" "), expected, "{scope:?} of {anchor}");
        }
    }
}
