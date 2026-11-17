use std::ops::RangeInclusive;

use protocol::PaneId;

use crate::close_scope::CloseScope;
use crate::group::Group;
use crate::strip_step::StripStep;

impl Group {
    /// How many members, from the front, are pinned.
    #[must_use]
    pub fn pinned_count(&self) -> usize {
        self.pinned
    }

    /// Whether `pane` is a pinned member.
    #[must_use]
    pub fn is_pinned(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes
            .iter()
            .take(self.pinned)
            .any(|member| member == pane)
    }

    /// The member a keyboard `step` from `from` lands on; `None` at either end of the strip,
    /// when already there, or when `from` is not a member.
    #[must_use]
    pub fn step(
        &self,
        from: &PaneId,
        step: StripStep,
    ) -> Option<&PaneId> {
        let at = self.panes.iter().position(|member| member == from)?;
        let target = match step {
            StripStep::Previous => at.checked_sub(1)?,
            StripStep::Next => at + 1,
            StripStep::First => 0,
            StripStep::Last => self.panes.len().saturating_sub(1),
        };
        self.panes.get(target).filter(|found| *found != from)
    }

    /// The members a bulk close started on `anchor` reaches. Pinned members are never in it,
    /// and neither is `anchor` itself unless the scope is [`CloseScope::All`].
    #[must_use]
    pub fn closable(
        &self,
        anchor: &PaneId,
        scope: CloseScope,
    ) -> Vec<PaneId> {
        let anchor_at = self.panes.iter().position(|member| member == anchor);
        self.panes
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
        let others = self.panes.len().saturating_sub(1);
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
        let Some(at) = self.panes.iter().position(|member| member == pane) else {
            return;
        };
        if at < self.pinned {
            return;
        }
        let moved = self.panes.remove(at);
        self.panes.insert(self.pinned, moved);
        self.pinned += 1;
    }

    pub(crate) fn unpin(
        &mut self,
        pane: &PaneId,
    ) {
        if !self.is_pinned(pane) {
            return;
        }
        let Some(at) = self.panes.iter().position(|member| member == pane) else {
            return;
        };
        let moved = self.panes.remove(at);
        self.pinned -= 1;
        self.panes.insert(self.pinned, moved);
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
            pinned: 0,
        }
    }

    fn pinned_group(
        names: &[&str],
        pinned: usize,
    ) -> Group {
        Group {
            pinned,
            ..group(names, names[0])
        }
    }

    fn names(group: &Group) -> Vec<String> {
        group.panes().iter().map(ToString::to_string).collect()
    }

    #[test]
    fn pinning_moves_the_pane_behind_the_pinned_ones_and_unpinning_in_front_of_the_rest() {
        let mut stack = group(&["a", "b", "c", "d"], "a");
        stack.pin(&id("c"));
        assert_eq!(names(&stack), ["c", "a", "b", "d"], "c pinned");
        stack.pin(&id("d"));
        assert_eq!(names(&stack), ["c", "d", "a", "b"], "d pinned after c");
        assert_eq!(stack.pinned_count(), 2, "count");
        stack.pin(&id("d"));
        assert_eq!(names(&stack), ["c", "d", "a", "b"], "pinning twice is a no-op");
        stack.unpin(&id("c"));
        assert_eq!(names(&stack), ["d", "c", "a", "b"], "c unpinned first of the rest");
        stack.unpin(&id("b"));
        assert_eq!(names(&stack), ["d", "c", "a", "b"], "unpinning a loose pane is a no-op");
        assert_eq!(stack.active(), &id("a"), "the shown pane never changes");
        assert!(stack.is_pinned(&id("d")) && !stack.is_pinned(&id("c")), "flags");
    }

    #[test]
    fn a_new_pane_never_lands_among_the_pinned_ones() {
        let cases = [
            ("front", Some(0), ["a", "b", "x", "c"]),
            ("inside", Some(1), ["a", "b", "x", "c"]),
            ("behind", Some(2), ["a", "b", "x", "c"]),
            ("end", None, ["a", "b", "c", "x"]),
        ];
        for (name, index, expected) in cases {
            let mut stack = pinned_group(&["a", "b", "c"], 2);
            stack.insert(id("x"), index).unwrap();
            assert_eq!(names(&stack), expected, "{name}");
            assert_eq!(stack.pinned_count(), 2, "{name}: pins kept");
        }
    }

    #[test]
    fn a_pane_reorders_only_inside_its_own_side_of_the_pin_line() {
        let cases = [
            ("pinned to the end clamps", "a", Some(3), ["b", "a", "c", "d", "e"]),
            ("pinned to the front", "b", Some(0), ["b", "a", "c", "d", "e"]),
            ("loose to the front clamps", "e", Some(0), ["a", "b", "e", "c", "d"]),
            ("loose to the end", "c", None, ["a", "b", "d", "e", "c"]),
        ];
        for (name, moved, index, expected) in cases {
            let mut stack = pinned_group(&["a", "b", "c", "d", "e"], 2);
            stack.reorder(&id(moved), index).unwrap();
            assert_eq!(names(&stack), expected, "{name}");
            assert_eq!(stack.pinned_count(), 2, "{name}: pins kept");
        }
    }

    #[test]
    fn taking_a_pinned_pane_shrinks_the_pinned_run_and_a_loose_one_leaves_it() {
        let mut stack = pinned_group(&["a", "b", "c"], 2);
        stack.take(&id("c"));
        assert_eq!(stack.pinned_count(), 2, "loose pane taken");
        stack.take(&id("a"));
        assert_eq!(stack.pinned_count(), 1, "pinned pane taken");
    }

    #[test]
    fn stepping_walks_the_strip_without_wrapping() {
        let stack = group(&["a", "b", "c"], "b");
        let cases = [
            ("previous", "b", StripStep::Previous, Some("a")),
            ("next", "b", StripStep::Next, Some("c")),
            ("before the first", "a", StripStep::Previous, None),
            ("after the last", "c", StripStep::Next, None),
            ("first", "c", StripStep::First, Some("a")),
            ("last", "a", StripStep::Last, Some("c")),
            ("first of the first", "a", StripStep::First, None),
            ("a stranger", "z", StripStep::Next, None),
        ];
        for (name, from, step, expected) in cases {
            let got = stack.step(&id(from), step).map(ToString::to_string);
            assert_eq!(got.as_deref(), expected, "{name}");
        }
    }

    #[test]
    fn closable_panes_leave_out_the_pinned_ones_and_the_pane_itself() {
        let stack = pinned_group(&["p", "a", "b", "c"], 1);
        let cases = [
            ("others", "b", CloseScope::Others, vec!["a", "c"]),
            ("to the right", "a", CloseScope::ToRight, vec!["b", "c"]),
            ("all", "a", CloseScope::All, vec!["a", "b", "c"]),
            ("to the right of the last", "c", CloseScope::ToRight, vec![]),
            ("to the right of a pinned one", "p", CloseScope::ToRight, vec!["a", "b", "c"]),
        ];
        for (name, anchor, scope, expected) in cases {
            let got: Vec<String> = stack
                .closable(&id(anchor), scope)
                .iter()
                .map(ToString::to_string)
                .collect();
            assert_eq!(got, expected, "{name}");
        }
    }

    #[test]
    fn a_pin_run_longer_than_the_strip_is_inconsistent() {
        assert!(pinned_group(&["a", "b"], 2).is_consistent(), "all pinned");
        assert!(!pinned_group(&["a", "b"], 3).is_consistent(), "too many");
    }
}
