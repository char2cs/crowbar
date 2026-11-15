//! What the user has typed or ticked in the merge box and not yet sent.

use std::collections::BTreeSet;

use gpui::{AppContext as _, Context, Entity};
use ui::{Surface, TextInput};

use crate::after_merge::AfterMerge;

const PLACEHOLDER: &str = "Describe the change…";

#[derive(Debug)]
pub(crate) struct MergeDraft {
    pub(crate) after: AfterMerge,
    pub(crate) message: Entity<TextInput>,
    pub(crate) skipped: BTreeSet<String>,
}

impl MergeDraft {
    pub(crate) fn new<T: 'static>(cx: &mut Context<'_, T>) -> Self {
        Self {
            after: AfterMerge::default(),
            message: cx.new(|cx| TextInput::new(PLACEHOLDER, cx).on(Surface::Chrome)),
            skipped: BTreeSet::new(),
        }
    }

    pub(crate) fn paths(
        &self,
        uncommitted: &[String],
    ) -> Vec<String> {
        uncommitted
            .iter()
            .filter(|path| !self.skipped.contains(*path))
            .cloned()
            .collect()
    }

    pub(crate) fn toggle(
        &mut self,
        path: &str,
    ) {
        if !self.skipped.remove(path) {
            self.skipped.insert(path.to_owned());
        }
    }
}

pub(crate) fn commit_label(count: usize) -> String {
    match count {
        1 => "Commit 1 change".to_owned(),
        _ => format!("Commit {count} changes"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_commit_button_counts_the_ticked_changes() {
        let cases = [
            (0, "Commit 0 changes"),
            (1, "Commit 1 change"),
            (4, "Commit 4 changes"),
        ];
        for (count, label) in cases {
            assert_eq!(commit_label(count), label);
        }
    }

    #[gpui::test]
    fn unticking_a_path_leaves_it_out_and_ticking_again_restores_it(cx: &mut gpui::TestAppContext) {
        use crate::test_support::prelude::*;
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let all = vec!["a.rs".to_owned(), "b.rs".to_owned()];
        sidebar.update(cx, |this, _| {
            this.merge.toggle("a.rs");
            assert_eq!(this.merge.paths(&all), ["b.rs"]);
            this.merge.toggle("a.rs");
            assert_eq!(this.merge.paths(&all), all);
        });
    }
}
