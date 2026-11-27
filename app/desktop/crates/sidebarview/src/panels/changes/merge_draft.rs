use std::collections::BTreeSet;

use gpui::{AppContext as _, Context, Entity, Subscription, px};
use ui::{Pose, Surface, TextArea, TextAreaSize, TextInputEvent};

use sidebar::AfterMerge;

const PLACEHOLDER: &str = "Describe the change…";
const MESSAGE_MAX_HEIGHT: f32 = 160.0;

#[derive(Debug)]
pub(crate) struct MergeDraft {
    pub(crate) after: AfterMerge,
    pub(crate) message: Entity<TextArea>,
    pub(crate) skipped: BTreeSet<String>,
    _submit: Subscription,
}

type Commit<T> = fn(&mut T, &mut Context<'_, T>);

fn submit_on<T: 'static>(
    event: &TextInputEvent,
    commit: Commit<T>,
    this: &mut T,
    cx: &mut Context<'_, T>,
) {
    if matches!(event, TextInputEvent::Changed(_)) {
        cx.notify();
    }
    if matches!(event, TextInputEvent::Submitted) {
        commit(this, cx);
    }
}

impl MergeDraft {
    pub(crate) fn new<T: 'static>(
        cx: &mut Context<'_, T>,
        commit: Commit<T>,
    ) -> Self {
        let message = cx.new(|cx| {
            TextArea::new(PLACEHOLDER, cx)
                .on(Surface::Chrome)
                .size(TextAreaSize::Small)
                .max_height(px(MESSAGE_MAX_HEIGHT))
        });
        let submit = cx.subscribe(&message, move |this, _, event: &TextInputEvent, cx| {
            submit_on(event, commit, this, cx);
        });
        Self {
            after: AfterMerge::default(),
            message,
            skipped: BTreeSet::new(),
            _submit: submit,
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

pub(crate) fn after_merge(delete: Pose) -> AfterMerge {
    if delete == Pose::On {
        return AfterMerge::DeleteBranch;
    }
    AfterMerge::Keep
}

#[cfg(test)]
mod tests {

    #[gpui::test]
    fn unticking_a_path_leaves_it_out_and_ticking_again_restores_it(cx: &mut gpui::TestAppContext) {
        use crate::test_support::prelude::*;
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let all = vec!["a.rs".to_owned(), "b.rs".to_owned()];
        sidebar.update(cx, |this, _| {
            this.merge.toggle("a.rs");
            assert_eq!(this.merge.paths(&all), ["b.rs"]);
            this.merge.toggle("a.rs");
            assert_eq!(this.merge.paths(&all), all);
        });
    }

    #[test]
    fn the_delete_checkbox_maps_to_the_choice() {
        use super::{AfterMerge, Pose, after_merge};
        assert_eq!(after_merge(Pose::On), AfterMerge::DeleteBranch);
        assert_eq!(after_merge(Pose::Off), AfterMerge::Keep);
    }
}
