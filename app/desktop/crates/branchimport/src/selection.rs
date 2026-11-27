use std::collections::BTreeSet;

#[derive(Debug, Default)]
pub(crate) struct Selection {
    picked: BTreeSet<String>,
    locked: BTreeSet<String>,
}

impl Selection {
    pub(crate) fn toggle(
        &mut self,
        name: &str,
    ) {
        if self.picked.remove(name) {
            self.locked.remove(name);
            return;
        }
        self.picked.insert(name.to_owned());
    }

    pub(crate) fn toggle_lock(
        &mut self,
        name: &str,
    ) {
        if self.locked.remove(name) {
            return;
        }
        self.locked.insert(name.to_owned());
    }

    pub(crate) fn is_picked(
        &self,
        name: &str,
    ) -> bool {
        self.picked.contains(name)
    }

    pub(crate) fn is_locked(
        &self,
        name: &str,
    ) -> bool {
        self.locked.contains(name)
    }

    pub(crate) fn picked(&self) -> Vec<String> {
        self.picked.iter().cloned().collect()
    }

    pub(crate) fn locked(&self) -> Vec<String> {
        self.locked
            .iter()
            .filter(|name| self.picked.contains(*name))
            .cloned()
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Clone, Copy)]
    enum Step {
        Pick(&'static str),
        Lock(&'static str),
    }
    use Step::{Lock, Pick};

    impl Step {
        fn apply(
            self,
            selection: &mut Selection,
        ) {
            match self {
                Pick(branch) => selection.toggle(branch),
                Lock(branch) => selection.toggle_lock(branch),
            }
        }
    }

    type Case = (
        &'static str,
        &'static [Step],
        &'static [&'static str],
        &'static [&'static str],
    );

    const CASES: [Case; 4] = [
        (
            "a pick is listed",
            &[Pick("a"), Pick("b")],
            &["a", "b"],
            &[],
        ),
        (
            "a pick is undone by picking again",
            &[Pick("a"), Pick("a")],
            &[],
            &[],
        ),
        (
            "a lock of a picked branch is kept",
            &[Pick("a"), Lock("a")],
            &["a"],
            &["a"],
        ),
        (
            "unpicking drops the lock and a re-pick starts unlocked",
            &[Pick("a"), Lock("a"), Pick("a"), Pick("a")],
            &["a"],
            &[],
        ),
    ];

    fn run(steps: &[Step]) -> Selection {
        let mut selection = Selection::default();
        for step in steps {
            step.apply(&mut selection);
        }
        selection
    }

    #[test]
    fn picks_and_locks_follow_the_steps_taken() {
        for (name, steps, picked, locked) in CASES {
            let selection = run(steps);
            assert_eq!(selection.picked(), picked, "{name}");
            assert_eq!(selection.locked(), locked, "{name}");
        }
    }

    #[test]
    fn a_lock_of_an_unpicked_branch_is_not_imported() {
        let mut selection = Selection::default();
        selection.toggle_lock("a");
        assert!(selection.is_locked("a") && !selection.is_picked("a"));
        assert_eq!(selection.locked().len(), 0);
    }
}
