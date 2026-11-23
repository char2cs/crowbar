use editor::Draft;
use gpui::App;
use protocol::PaneId;

use super::followed::Followed;
use super::landed::Landed;
use super::{EditorPanes, EditorTarget};

impl EditorPanes {
    pub(crate) fn keep_draft(
        &self,
        pane: &PaneId,
        cx: &App,
    ) {
        let draft = self
            .panes
            .borrow()
            .get(pane)
            .and_then(|followed| self.draft_of(followed, cx));
        let Some(draft) = draft else {
            return;
        };
        if let Err(error) = self.drafts.borrow_mut().keep(draft) {
            tracing::warn!(%error, %pane, "an unsaved draft was not written");
        }
    }

    fn draft_of(
        &self,
        followed: &Followed,
        cx: &App,
    ) -> Option<Draft> {
        let view = followed.view()?.read(cx);
        let uri = self.uri_of(&followed.target)?;
        let text = view.text(cx);
        let saved_text = view.saved_text();
        (text != saved_text).then_some(Draft {
            uri,
            text,
            saved_text,
        })
    }

    pub(crate) fn discard_draft(
        &self,
        pane: &PaneId,
    ) {
        let target = self
            .panes
            .borrow()
            .get(pane)
            .map(|followed| followed.target.clone());
        if let Some(target) = target {
            self.forget_drafts(&[target]);
        }
    }

    pub(super) fn forget_drafts(
        &self,
        targets: &[EditorTarget],
    ) {
        let mut drafts = self.drafts.borrow_mut();
        let uris = targets.iter().filter_map(|target| self.uri_of(target));
        let failed = uris.filter_map(|uri| drafts.discard(&uri).err().map(|error| (uri, error)));
        for (uri, error) in failed.collect::<Vec<_>>() {
            tracing::warn!(%error, %uri, "a discarded draft stayed on disk");
        }
    }

    pub(super) fn landed(
        &self,
        target: &EditorTarget,
        disk: String,
    ) -> Landed {
        let Some(uri) = self.uri_of(target) else {
            return Landed { disk, draft: None };
        };
        let draft = self.drafts.borrow().get(&uri).cloned();
        let draft = draft.filter(|draft| draft.unsaved(Some(&disk)));
        if draft.is_none() {
            self.forget_drafts(std::slice::from_ref(target));
        }
        Landed { disk, draft }
    }

    fn uri_of(
        &self,
        target: &EditorTarget,
    ) -> Option<String> {
        let file = target.file.as_ref()?;
        if file.is_host() {
            return Some(editor::host_uri(file.as_str()));
        }
        let workspace = (self.chats.workspaces)(&target.chat)?;
        Some(editor::workspace_uri(&workspace, file.as_str()))
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::super::tests::{Opened, PANE, PATH, TEXT, opened, typed_and_kept};
    use super::*;
    use crate::test_support::FakeDocuments;

    type Ending<'a> = dyn Fn(&EditorPanes) + 'a;

    #[gpui::test]
    fn a_draft_goes_once_its_file_is_saved_or_its_tab_closes(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let pane = PaneId::new(PANE).unwrap();
        let saved = |panes: &EditorPanes| panes.discard_draft(&pane);
        let closed = |panes: &EditorPanes| panes.follow(&layout::Layout::default());
        let ends: [(&str, &Ending<'_>); 2] = [("saved", &saved), ("closed", &closed)];
        for (end, close) in ends {
            let documents = Rc::new(FakeDocuments::default());
            documents.loading("c1");
            documents.holding(PATH, TEXT);
            let panes = typed_and_kept(&documents, cx);
            close(&panes);
            drop(panes);
            let Opened { view, window, .. } = opened(&documents, cx);
            assert_eq!(window.read(|cx| view.read(cx).text(cx)), TEXT, "{end}");
        }
    }
}
