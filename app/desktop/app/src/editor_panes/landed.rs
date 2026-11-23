use editorview::{Access, Opening, SaveMark};
use protocol::PaneId;

use super::editor_target::EditorTarget;
use crate::document_request::DocumentRequest;
use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

#[derive(Debug, Default)]
pub(crate) struct Landed {
    pub(crate) disk: String,
    pub(crate) draft: Option<editor::Draft>,
}

impl Landed {
    pub(super) fn opening(
        &self,
        target: &EditorTarget,
        access: Access,
    ) -> Opening {
        let (text, saved) = match &self.draft {
            Some(draft) => (draft.text.clone(), Some(draft.saved_text.clone())),
            None => (self.disk.clone(), None),
        };
        Opening {
            text,
            path: target.label.clone(),
            line: target.line,
            access,
            saved,
        }
    }

    pub(super) fn restored(
        &self,
        since: SaveMark,
        (pane, requests): (&PaneId, &Requests),
    ) {
        let Some(draft) = &self.draft else {
            return;
        };
        let dirty = DocumentRequest::Dirty {
            pane: pane.clone(),
            dirty: true,
        };
        send(requests, ShellRequest::Document(dirty));
        if draft.saved_text == self.disk {
            return;
        }
        let (pane, text) = (pane.clone(), self.disk.clone());
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
            window.read(|cx| view.read(cx).is_dirty(cx)),
            "restored dirty"
        );
        let since = window.read(|cx| view.read(cx).save_mark());
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
}
