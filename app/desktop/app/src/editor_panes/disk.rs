use editorview::{DiskTake, SaveMark};
use gpui::{App, Window};
use protocol::{DocumentPath, PaneId};

use super::EditorPanes;
use super::followed::Followed;
use crate::document_source::disk_into;

impl EditorPanes {
    pub(crate) fn disk_changed(
        &self,
        (workspace, path): (&str, Option<&str>),
        cx: &App,
    ) {
        let reads: Vec<(PaneId, String, DocumentPath, SaveMark)> = self
            .panes
            .borrow()
            .iter()
            .filter(|(_, followed)| self.touches(followed, (workspace, path)))
            .filter_map(|(pane, followed)| {
                let file = followed.target.file.clone()?;
                let since = followed.view()?.read(cx).save_mark();
                Some((pane.clone(), followed.target.chat.clone(), file, since))
            })
            .collect();
        for read in reads {
            self.read_again(read);
        }
    }

    fn read_again(
        &self,
        (pane, chat, file, since): (PaneId, String, DocumentPath, SaveMark),
    ) {
        let done = disk_into(self.requests.clone(), pane.clone(), since);
        if let Err(error) = self.documents.read(&chat, &file, done) {
            tracing::debug!(%error, %pane, "a changed file could not be read again");
        }
    }

    pub(crate) fn take_disk(
        &self,
        (pane, read): (&PaneId, (&str, SaveMark)),
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
        view.update(cx, |view, cx| view.take_disk_text(read, window, cx))
    }

    pub(crate) fn file_name(
        &self,
        pane: &PaneId,
    ) -> Option<String> {
        let panes = self.panes.borrow();
        let file = panes.get(pane)?.target.file.as_ref()?.as_str();
        Some(file.rsplit('/').next().unwrap_or(file).to_owned())
    }

    fn touches(
        &self,
        followed: &Followed,
        (workspace, path): (&str, Option<&str>),
    ) -> bool {
        let Some(file) = &followed.target.file else {
            return false;
        };
        let named = path.is_none_or(|path| file.as_str() == path);
        named
            && !file.is_host()
            && (self.chats.workspaces)(&followed.target.chat).as_deref() == Some(workspace)
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;
    use serde_json::json;

    use super::super::tests::{PANE, PATH, TEXT, landed, layout_with, panes_over, shown_in};
    use super::*;
    use crate::document_request::DocumentRequest;
    use crate::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    #[gpui::test]
    fn a_file_changed_in_its_workspace_reads_again_and_a_clean_view_shows_disk(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        landed(&panes, &heard);
        let (view, window) = shown_in(&panes, cx);
        documents.holding(PATH, "changed\n");
        let changes = [
            ("w2", Some(PATH)),
            ("w1", Some("src/other.ts")),
            ("w1", Some(PATH)),
        ];
        for change in changes {
            window.update(|_, cx| panes.disk_changed(change, cx));
        }
        let Ok(ShellRequest::Document(DocumentRequest::DiskRead { pane, text, since })) =
            heard.try_recv()
        else {
            panic!("the changed file reads again");
        };
        assert!(
            heard.try_recv().is_err(),
            "only its own file in its own workspace"
        );
        assert_eq!(pane, PaneId::new(PANE).unwrap());
        let taken =
            window.update(|window, cx| panes.take_disk((&pane, (&text, since)), window, cx));
        assert_eq!(taken, DiskTake::Taken);
        assert_eq!(
            view.read_with(window, editorview::EditorView::text),
            "changed\n"
        );
    }
}
