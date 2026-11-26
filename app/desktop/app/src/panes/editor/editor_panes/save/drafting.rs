use std::collections::BTreeSet;

use editor::Draft;
use gpui::App;
use protocol::PaneId;

use crate::panes::editor::editor_panes::load::followed::Followed;
use crate::panes::editor::editor_panes::load::landed::Landed;
use crate::panes::editor::editor_panes::{EditorPanes, EditorTarget, Sweep};

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
        let document = followed.document(cx)?;
        let document = document.read(cx);
        let uri = self.uri_of(&followed.target)?;
        let text = document.text(cx);
        let saved_text = document.saved_text();
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

    pub(crate) fn forget_drafts(
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

    pub(crate) fn landed(
        &self,
        target: &EditorTarget,
        read: Result<String, editor::OpenFailure>,
    ) -> Result<Landed, editor::OpenFailure> {
        let draft = self.draft_over(target, read.as_deref().ok());
        match (read, draft) {
            (Ok(disk), draft) => Ok(Landed {
                disk: Some(disk),
                draft,
            }),
            (Err(failure), None) => Err(failure),
            (Err(_), draft) => Ok(Landed { disk: None, draft }),
        }
    }

    pub(crate) fn forget_released(
        &self,
        gone: &[EditorTarget],
    ) {
        let held = self.held_uris();
        let released: Vec<EditorTarget> = gone
            .iter()
            .filter(|target| self.uri_of(target).is_none_or(|uri| !held.contains(&uri)))
            .cloned()
            .collect();
        for uri in released.iter().filter_map(|target| self.uri_of(target)) {
            self.source_shown.forget(&uri);
        }
        self.forget_drafts(&released);
    }

    fn held_uris(&self) -> BTreeSet<String> {
        self.panes
            .borrow()
            .values()
            .filter(|followed| followed.target.preview.is_none())
            .filter_map(|followed| self.uri_of(&followed.target))
            .collect()
    }

    fn draft_over(
        &self,
        target: &EditorTarget,
        disk: Option<&str>,
    ) -> Option<Draft> {
        let uri = self.uri_of(target).filter(|_| target.preview.is_none())?;
        let draft = self.drafts.borrow().get(&uri).cloned();
        let draft = draft.filter(|draft| draft.unsaved(disk));
        if draft.is_none() {
            self.forget_drafts(std::slice::from_ref(target));
        }
        draft
    }

    pub(crate) fn sweep_drafts(&self) {
        if self.sweep.get() == Sweep::Done {
            return;
        }
        let held: Option<BTreeSet<String>> = self
            .panes
            .borrow()
            .values()
            .filter(|followed| followed.target.file.is_some() && followed.target.preview.is_none())
            .map(|followed| self.uri_of(&followed.target))
            .collect();
        let Some(held) = held else {
            return;
        };
        self.sweep.set(Sweep::Done);
        if let Err(error) = self.drafts.borrow_mut().keep_only(&held) {
            tracing::warn!(%error, "the drafts of tabs not restored stayed on disk");
        }
    }

    pub(crate) fn uri_of(
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

    use serde_json::json;

    use super::*;
    use crate::panes::editor::editor_panes::Landing;
    use crate::panes::editor::editor_panes::tests::editors_in;
    use crate::panes::editor::editor_panes::tests::{
        Opened, PANE, PATH, TEXT, landed, layout_in, opened, panes_over, shown_in, two_panes_on,
        typed_and_kept,
    };
    use crate::test_support::{FakeDocuments, memory_drafts};

    type Ending<'a> = dyn Fn(&EditorPanes) + 'a;

    #[gpui::test]
    fn a_draft_goes_once_its_file_is_saved_or_its_tab_closes(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let pane = PaneId::new(PANE).unwrap();
        let saved = |panes: &EditorPanes| panes.discard_draft(&pane);
        let closed = |panes: &EditorPanes| panes.follow(&layout::Layout::default(), "");
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

    fn kept(documents: &FakeDocuments) -> bool {
        let uri = editor::workspace_uri("w1", PATH);
        memory_drafts(&documents.drafts()).get(&uri).is_some()
    }

    #[gpui::test]
    fn a_draft_goes_at_startup_once_no_restored_tab_holds_it(cx: &mut TestAppContext) {
        let documents = editors_in(cx);
        documents.holding(PATH, TEXT);
        drop(typed_and_kept(&documents, cx));
        let (panes, _heard) = panes_over(&documents);
        panes.follow(&layout_in("c2", json!({"path": PATH})), "");
        assert!(kept(&documents), "its tab's workspace is not known yet");
        documents.loading("c2");
        cx.update(|cx| panes.data_changed(cx));
        drop(panes);
        assert!(
            kept(&documents),
            "a tab of another chat on its workspace holds it"
        );
        let (panes, _heard) = panes_over(&documents);
        panes.follow(&layout::Layout::default(), "");
        assert!(!kept(&documents), "no tab holds it");
    }

    #[gpui::test]
    fn closing_one_of_two_panes_on_a_file_keeps_its_draft(cx: &mut TestAppContext) {
        let documents = editors_in(cx);
        documents.loading("c2");
        documents.holding(PATH, TEXT);
        let panes = typed_and_kept(&documents, cx);
        assert!(kept(&documents), "typed");
        panes.follow(&two_panes_on(PATH), "");
        assert!(kept(&documents), "a second pane opens on it");
        panes.follow(&layout_in("c1", json!({"path": PATH})), "");
        assert!(kept(&documents), "the other pane still shows the file");
        panes.follow(&layout::Layout::default(), "");
        assert!(!kept(&documents), "no pane shows it");
    }

    #[gpui::test]
    fn a_draft_opens_when_its_file_cannot_be_read(cx: &mut TestAppContext) {
        let documents = editors_in(cx);
        documents.holding(PATH, TEXT);
        drop(typed_and_kept(&documents, cx));
        documents.refusing(PATH, 404);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_in("c1", json!({"path": PATH})), "");
        assert_eq!(landed(&panes, &heard), Landing::Shown, "the draft opens");
        let (view, window) = shown_in(&panes, cx);
        assert_eq!(window.read(|cx| view.read(cx).text(cx)), format!("x{TEXT}"));
        assert!(window.read(|cx| view.read(cx).document().read(cx).is_dirty(cx)));
    }
}
