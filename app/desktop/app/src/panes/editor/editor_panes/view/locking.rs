use gpui::App;

use crate::panes::editor::editor_panes::EditorPanes;

impl EditorPanes {
    pub(crate) fn follow_locks(
        &self,
        cx: &mut App,
    ) {
        for followed in self.panes.borrow_mut().values_mut() {
            let access = self.access_of(&followed.target);
            followed.set_access(access, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Focusable as _, TestAppContext};
    use serde_json::json;

    use crate::panes::editor::editor_panes::Landing;
    use crate::panes::editor::editor_panes::tests::{
        PATH, TEXT, landed, layout_with, panes_locked, shown_in,
    };
    use crate::test_support::FakeDocuments;

    #[gpui::test]
    fn an_open_file_follows_its_workspace_locking_and_unlocking(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let locked = Rc::new(Cell::new(false));
        let lock = Rc::clone(&locked);
        let (panes, heard) = panes_locked(&documents, Rc::new(move |_| lock.get()));
        panes.follow(&layout_with(json!({"path": PATH})), "");
        assert_eq!(landed(&panes, &heard), Landing::Shown);
        let (view, window) = shown_in(&panes, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        let mut expected = TEXT.to_owned();
        for (lock_now, typed) in [(true, None), (false, Some('y')), (true, None)] {
            locked.set(lock_now);
            window.update(|_, cx| panes.data_changed(cx));
            window.simulate_input("y");
            expected = typed.map_or(expected.clone(), |key| format!("{key}{expected}"));
            let text = window.read(|cx| view.read(cx).text(cx));
            assert_eq!(text, expected, "locked {lock_now}");
        }
    }
}
