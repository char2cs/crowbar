use gpui::{Context, Window};

use super::EditorView;

impl EditorView {
    /// Text the pane's rich view wrote: one undoable edit that keeps the carets, dirty against
    /// the saved text as typing is. The same text again changes nothing.
    pub fn take_page_text(
        &mut self,
        text: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.state.read(cx).text() == text {
            return;
        }
        self.state.update(cx, |state, cx| {
            state.replace_all_keeping_selections(text.to_owned(), window, cx);
        });
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::editor_view::test_support::{listened, opened};
    use crate::{Access, EditorEvent};

    fn take_text(
        view: &gpui::Entity<EditorView>,
        window: &mut gpui::VisualTestContext,
    ) {
        view.update_in(window, |view, window, cx| {
            view.take_page_text("a b", window, cx);
        });
    }

    #[gpui::test]
    fn the_rich_view_s_text_dirties_the_view_once_and_undoes_in_one_step(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let heard = listened(&view, window);
        take_text(&view, window);
        take_text(&view, window);
        assert_eq!(view.read_with(window, EditorView::text), "a b");
        assert_eq!(*heard.borrow(), [EditorEvent::DirtyChanged(true)]);
        window.simulate_keystrokes("ctrl-z");
        assert_eq!(
            view.read_with(window, EditorView::text),
            "a",
            "one undo step"
        );
    }
}
