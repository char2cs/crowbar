use gpui::Context;

use super::EditorView;
use crate::Access;

impl EditorView {
    /// Typing edits the view again, or stops editing it, as its workspace unlocks or locks.
    pub fn set_access(
        &mut self,
        access: Access,
        cx: &mut Context<'_, Self>,
    ) {
        if self.access == access {
            return;
        }
        self.access = access;
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Focusable as _, TestAppContext};

    use super::*;
    use crate::editor_view::test_support::opened;

    #[gpui::test]
    fn typing_follows_the_access_the_view_is_given_last(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        let cases = [(Access::ReadOnly, "a"), (Access::Editable, "xa")];
        for (access, expected) in cases {
            window.update(|_, cx| view.update(cx, |view, cx| view.set_access(access, cx)));
            window.simulate_input("x");
            let text = window.read(|cx| view.read(cx).text(cx));
            assert_eq!(text, expected, "{access:?}");
        }
    }
}
