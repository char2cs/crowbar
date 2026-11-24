use gpui::{App, Focusable as _, Window};
use protocol::PaneId;

use super::EditorPanes;
use super::keyboard::Keyboard;
use super::pane_view::PaneView;

impl EditorPanes {
    pub(crate) fn focus(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) {
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane).filter(|f| f.view == PaneView::Source) else {
            return;
        };
        match followed.view() {
            Some(view) => window.focus(&view.focus_handle(cx), cx),
            None => followed.keyboard = Keyboard::Take,
        }
    }
}

impl EditorPanes {
    pub(crate) fn reveal(
        &self,
        (pane, at): (&PaneId, (u32, u32)),
        window: &mut Window,
        cx: &mut App,
    ) {
        let view = self
            .panes
            .borrow()
            .get(pane)
            .and_then(|f| f.view().cloned());
        let Some(view) = view else {
            return;
        };
        view.update(cx, |view, cx| view.reveal_lsp_position(at, window, cx));
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;
    use serde_json::json;

    use super::super::tests::{PANE, PATH, TEXT, landed, layout_with, panes_over, shown_in};
    use super::*;
    use crate::editor_panes::Landing;
    use crate::test_support::FakeDocuments;

    #[gpui::test]
    fn a_file_pane_focused_as_it_opens_or_pressed_later_takes_the_keyboard(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        assert_eq!(landed(&panes, &heard), Landing::Shown);
        let (view, window) = shown_in(&panes, cx);
        let has_keys = |window: &mut gpui::VisualTestContext| {
            window.update(|window, cx| view.focus_handle(cx).is_focused(window))
        };
        assert!(has_keys(window), "focused as it opened");
        window.update(Window::blur);
        assert!(!has_keys(window), "the keyboard went elsewhere");
        let pane = PaneId::new(PANE).unwrap();
        window.update(|window, cx| panes.focus(&pane, window, cx));
        assert!(has_keys(window), "its tab was pressed");
    }
}
