use editorview::{Access, DocumentOpening, Opening, SaveMark};
use protocol::PaneId;

use super::editor_target::EditorTarget;
use crate::document_request::DocumentRequest;
use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

#[derive(Debug, Default)]
pub(crate) struct Landed {
    pub(crate) disk: Option<String>,
    pub(crate) draft: Option<editor::Draft>,
}

impl Landed {
    pub(super) fn opening(
        &self,
        target: &EditorTarget,
        access: Access,
    ) -> (DocumentOpening, Opening) {
        let (text, saved) = match &self.draft {
            Some(draft) => (draft.text.clone(), Some(draft.saved_text.clone())),
            None => (self.disk.clone().unwrap_or_default(), None),
        };
        let document = DocumentOpening {
            text,
            path: target.label.clone(),
            saved,
        };
        let opening = Opening {
            line: target.line,
            access,
        };
        (document, opening)
    }

    pub(super) fn text(&self) -> Option<String> {
        let draft = self.draft.as_ref().map(|draft| draft.text.clone());
        draft.or_else(|| self.disk.clone())
    }

    pub(super) fn tell_dirty(
        &self,
        (pane, requests): (&PaneId, &Requests),
    ) {
        if self.draft.is_none() {
            return;
        }
        let dirty = DocumentRequest::Dirty {
            pane: pane.clone(),
            dirty: true,
        };
        send(requests, ShellRequest::Document(dirty));
    }

    pub(super) fn restored(
        &self,
        since: SaveMark,
        (pane, requests): (&PaneId, &Requests),
    ) {
        let (Some(draft), Some(disk)) = (&self.draft, &self.disk) else {
            return;
        };
        if draft.saved_text == *disk {
            return;
        }
        let (pane, text) = (pane.clone(), disk.clone());
        let read = DocumentRequest::DiskRead { pane, text, since };
        send(requests, ShellRequest::Document(read));
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::super::tests::{Opened, PANE, PATH, TEXT, opened, typed_and_kept};
    use super::*;
    use crate::test_support::FakeDocuments;

    fn heard_all(heard: &async_channel::Receiver<ShellRequest>) -> Vec<ShellRequest> {
        std::iter::from_fn(|| heard.try_recv().ok()).collect()
    }

    #[gpui::test]
    fn a_dirty_file_comes_back_dirty_after_a_restart_and_says_if_disk_moved(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let pane = PaneId::new(PANE).unwrap();
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        drop(typed_and_kept(&documents, cx));
        documents.holding(PATH, "moved\n");
        let Opened {
            heard,
            view,
            window,
            ..
        } = opened(&documents, cx);
        let typed = format!("x{TEXT}");
        assert_eq!(window.read(|cx| view.read(cx).text(cx)), typed);
        assert!(
            window.read(|cx| view.read(cx).document().read(cx).is_dirty(cx)),
            "restored dirty"
        );
        let since = window.read(|cx| view.read(cx).document().read(cx).save_mark());
        let told = [
            ShellRequest::Document(DocumentRequest::Dirty {
                pane: pane.clone(),
                dirty: true,
            }),
            ShellRequest::Document(DocumentRequest::DiskRead {
                pane,
                text: "moved\n".to_owned(),
                since,
            }),
        ];
        assert_eq!(heard_all(&heard), told);
    }

    #[gpui::test]
    fn a_restored_dirty_file_says_so_before_its_tab_is_first_drawn(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        drop(typed_and_kept(&documents, cx));
        let (panes, heard) = super::super::tests::panes_over(&documents);
        panes.follow(
            &super::super::tests::layout_with(serde_json::json!({"path": PATH})),
            "",
        );
        super::super::tests::landed(&panes, &heard);
        let dirty = ShellRequest::Document(DocumentRequest::Dirty {
            pane: PaneId::new(PANE).unwrap(),
            dirty: true,
        });
        assert_eq!(heard_all(&heard), [dirty]);
    }
}
