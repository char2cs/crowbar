use editorview::DiskTake;
use gpui::{App, Window};
use protocol::PaneId;

use super::EditorPanes;
use super::followed::Followed;
use crate::document_request::DocumentRequest;
use crate::request_queue::send;
use crate::shell_request::ShellRequest;
use store::FileDone;

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/editor/lib/reload-buffer.ts
impl EditorPanes {
    pub(crate) fn reload(
        &self,
        pane: &PaneId,
        cx: &App,
    ) {
        if self.unavailable_target(pane).is_some() {
            return self.retry(pane);
        }
        let panes = self.panes.borrow();
        let Some(followed) = panes.get(pane) else {
            return;
        };
        let (Some(view), Some(file)) = (followed.view(), &followed.target.file) else {
            return;
        };
        let since = view.read(cx).save_mark();
        let done = reloaded_into(self.requests.clone(), pane.clone(), since);
        if let Err(error) = self.documents.read(&followed.target.chat, file, done) {
            tracing::debug!(%error, %pane, "a reloaded file could not be read");
        }
    }

    pub(crate) fn take_reloaded(
        &self,
        (pane, read): (&PaneId, (&str, editorview::SaveMark)),
        window: &mut Window,
        cx: &mut App,
    ) -> DiskTake {
        let view = self
            .panes
            .borrow()
            .get(pane)
            .and_then(Followed::view)
            .cloned();
        let Some(view) = view else {
            return DiskTake::Kept;
        };
        view.update(cx, |view, cx| view.take_reloaded_text(read, window, cx))
    }

    pub(crate) fn is_dirty(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> bool {
        self.panes
            .borrow()
            .get(pane)
            .and_then(Followed::view)
            .is_some_and(|view| view.read(cx).is_dirty(cx))
    }
}

fn reloaded_into(
    requests: async_channel::Sender<ShellRequest>,
    pane: PaneId,
    since: editorview::SaveMark,
) -> FileDone<String> {
    Box::new(move |result| match result {
        Ok(text) => send(
            &requests,
            ShellRequest::Document(DocumentRequest::Reloaded { pane, text, since }),
        ),
        Err(error) => tracing::debug!(%error, %pane, "a reloaded file did not read"),
    })
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Focusable as _, TestAppContext};
    use serde_json::json;

    use super::super::tests::{PANE, PATH, TEXT, landed, layout_with, panes_over, shown_in};
    use super::*;
    use crate::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    #[gpui::test]
    fn a_reload_reads_the_file_again_and_shows_it_as_saved_over_the_edits(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        landed(&panes, &heard);
        let (view, window) = shown_in(&panes, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        window.simulate_input("x");
        while heard.try_recv().is_ok() {}
        documents.holding(PATH, "changed\n");
        let pane = PaneId::new(PANE).unwrap();
        window.update(|_, cx| panes.reload(&pane, cx));
        let Ok(ShellRequest::Document(DocumentRequest::Reloaded { pane, text, since })) =
            heard.try_recv()
        else {
            panic!("the file reads again");
        };
        let taken =
            window.update(|window, cx| panes.take_reloaded((&pane, (&text, since)), window, cx));
        assert_eq!(taken, DiskTake::Taken);
        let shown = view.read_with(window, editorview::EditorView::text);
        assert_eq!(shown, "changed\n");
        assert!(!window.update(|_, cx| panes.is_dirty(&pane, cx)), "clean");
    }
}
