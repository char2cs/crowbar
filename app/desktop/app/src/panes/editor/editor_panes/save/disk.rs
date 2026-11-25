use editorview::{DiskTake, SaveMark};
use gpui::App;
use protocol::{DocumentPath, PaneId};

use crate::panes::editor::document_source::disk_into;
use crate::panes::editor::editor_panes::load::followed::Followed;
use crate::panes::editor::editor_panes::{EditorPanes, EditorTarget, PanePhase};

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
            .filter(|(pane, _)| self.leads(pane, cx))
            .filter_map(|(pane, followed)| {
                let file = followed.target.file.clone()?;
                let since = followed.document(cx)?.read(cx).save_mark();
                Some((pane.clone(), followed.target.chat.clone(), file, since))
            })
            .collect();
        for read in reads {
            self.read_again(read);
        }
        if path.is_none() {
            self.retry_unavailable(workspace);
        }
    }

    fn retry_unavailable(
        &self,
        workspace: &str,
    ) {
        let failed: Vec<(PaneId, EditorTarget)> = self
            .panes
            .borrow()
            .iter()
            .filter(|(_, followed)| matches!(followed.phase, PanePhase::Unavailable(_)))
            .filter(|(_, followed)| self.touches(followed, (workspace, None)))
            .map(|(pane, followed)| (pane.clone(), followed.target.clone()))
            .collect();
        for (pane, target) in failed {
            self.start_read(&pane, &target);
        }
    }

    pub(crate) fn disk_missing(
        &self,
        (pane, failure, since): (&PaneId, editor::OpenFailure, SaveMark),
        cx: &App,
    ) -> bool {
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane) else {
            return false;
        };
        let clean = followed.document(cx).is_some_and(|document| {
            let document = document.read(cx);
            document.save_mark() == since && !document.is_dirty(cx)
        });
        if !clean {
            return false;
        }
        followed.phase = PanePhase::Unavailable(failure);
        true
    }

    fn read_again(
        &self,
        (pane, chat, file, since): (PaneId, String, DocumentPath, SaveMark),
    ) {
        let done = disk_into(self.requests.clone(), pane.clone(), (file.clone(), since));
        if let Err(error) = self.documents.read(&chat, &file, done) {
            tracing::debug!(%error, %pane, "a changed file could not be read again");
        }
    }

    pub(crate) fn take_disk(
        &self,
        (pane, read): (&PaneId, (&str, SaveMark)),
        cx: &mut App,
    ) -> DiskTake {
        let Some(document) = self.document_of(pane, cx) else {
            return DiskTake::Kept;
        };
        document.update(cx, |document, cx| document.take_disk_text(read, cx))
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

    use super::*;
    use crate::panes::editor::document_request::DocumentRequest;
    use crate::panes::editor::editor_panes::Landing;
    use crate::panes::editor::editor_panes::tests::{
        Opened, PANE, PATH, TEXT, landed, layout_with, opened, panes_over, shown_in,
    };
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
        panes.follow(&layout_with(json!({"path": PATH})), "");
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
        let taken = window.update(|_, cx| panes.take_disk((&pane, (&text, since)), cx));
        assert_eq!(taken, DiskTake::Taken);
        assert_eq!(
            view.read_with(window, editorview::EditorView::text),
            "changed\n"
        );
    }

    fn gone_and_heard(
        documents: &FakeDocuments,
        (panes, heard): (&EditorPanes, &async_channel::Receiver<ShellRequest>),
        window: &mut gpui::VisualTestContext,
    ) -> bool {
        documents.refusing(PATH, 404);
        window.update(|_, cx| panes.disk_changed(("w1", None), cx));
        let Ok(ShellRequest::Document(DocumentRequest::DiskMissing {
            pane,
            failure,
            since,
        })) = heard.try_recv()
        else {
            return false;
        };
        window.update(|_, cx| panes.disk_missing((&pane, failure, since), cx))
    }

    #[gpui::test]
    fn a_clean_file_gone_from_disk_is_unavailable_until_a_sync_finds_it_again(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let Opened {
            panes,
            heard,
            window,
            ..
        } = opened(&documents, cx);
        assert!(gone_and_heard(&documents, (&panes, &heard), window));
        let pane = PaneId::new(PANE).unwrap();
        let unavailable = |panes: &EditorPanes| {
            let panes = panes.panes.borrow();
            matches!(
                panes.get(&pane).map(|f| &f.phase),
                Some(PanePhase::Unavailable(_))
            )
        };
        assert!(unavailable(&panes), "the tab says the file is gone");
        documents.holding(PATH, TEXT);
        window.update(|_, cx| panes.disk_changed(("w1", None), cx));
        assert_eq!(
            landed(&panes, &heard),
            Landing::Shown,
            "back on the next sync"
        );
    }

    #[gpui::test]
    fn a_dirty_file_gone_from_disk_keeps_its_edits(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let Opened {
            panes,
            heard,
            window,
            ..
        } = opened(&documents, cx);
        window.simulate_input("x");
        assert!(!gone_and_heard(&documents, (&panes, &heard), window));
    }
}
