use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use editorview::Access;
use gpui::{AnyElement, App, Window};
use layout::{Layout, Tab};
use protocol::PaneId;

use crate::document_source::{DocumentSource, answer_into};
use crate::shell_request::ShellRequest;

mod arrival;
mod chat_reads;
mod disk;
mod drafting;
mod editor_target;
mod focusing;
mod followed;
mod keyboard;
mod landed;
mod landing;
mod locking;
mod pane_phase;
mod retry;
mod saving;
mod saving_as;
mod unavailable_panel;

pub(crate) use chat_reads::ChatReads;

pub(crate) const DRAFTS_FILE: &str = "editor-drafts.json";
pub(crate) use editor_target::EditorTarget;
use followed::Followed;
use keyboard::Keyboard;
pub(crate) use landing::Landing;
use pane_phase::PanePhase;

pub(crate) struct EditorPanes {
    documents: Rc<dyn DocumentSource>,
    chats: ChatReads,
    drafts: RefCell<editor::Drafts>,
    panes: RefCell<BTreeMap<PaneId, Followed>>,
    requests: async_channel::Sender<ShellRequest>,
}

impl EditorPanes {
    pub(crate) fn new(
        documents: Rc<dyn DocumentSource>,
        chats: ChatReads,
        drafts: editor::Drafts,
        requests: async_channel::Sender<ShellRequest>,
    ) -> Self {
        Self {
            documents,
            chats,
            drafts: RefCell::new(drafts),
            panes: RefCell::default(),
            requests,
        }
    }

    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.borrow().contains_key(pane)
    }

    pub(crate) fn follow(
        &self,
        layout: &Layout,
    ) {
        let open: BTreeMap<PaneId, EditorTarget> = layout
            .tabs()
            .iter()
            .flat_map(Tab::pane_ids)
            .filter_map(|pane| Some((pane.clone(), EditorTarget::of(layout.pane(&pane)?)?)))
            .collect();
        let focused = layout.active_tab().and_then(Tab::focused);
        let mut panes = self.panes.borrow_mut();
        let gone: Vec<EditorTarget> = panes
            .iter()
            .filter(|(pane, followed)| {
                !open.get(*pane).is_some_and(|target| followed.shows(target))
            })
            .map(|(_, followed)| followed.target.clone())
            .collect();
        panes.retain(|pane, followed| open.get(pane).is_some_and(|target| followed.shows(target)));
        for (pane, target) in open {
            let access = self.access_of(&target);
            let keyboard = focused
                .filter(|focused| **focused == pane && access == Access::Editable)
                .map_or(Keyboard::Leave, |_| Keyboard::Take);
            panes
                .entry(pane)
                .or_insert_with(|| Followed::arrive(target, (keyboard, access)));
        }
        drop(panes);
        self.forget_drafts(&gone);
        self.read_waiting();
    }

    pub(crate) fn data_changed(
        &self,
        cx: &mut App,
    ) {
        self.read_waiting();
        self.follow_locks(cx);
    }

    pub(crate) fn retry(
        &self,
        pane: &PaneId,
    ) {
        let Some(target) = self.unavailable_target(pane) else {
            return;
        };
        self.start_read(pane, &target);
    }

    pub(crate) fn land(
        &self,
        pane: &PaneId,
        read: Result<String, editor::OpenFailure>,
    ) -> Landing {
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane) else {
            return Landing::Quiet;
        };
        let landed = read.map(|disk| self.landed(&followed.target, disk));
        followed.land(landed)
    }

    pub(crate) fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<AnyElement> {
        let mut panes = self.panes.borrow_mut();
        let followed = panes.get_mut(pane)?;
        Some(followed.drawn((pane, &self.requests), window, cx))
    }

    fn access_of(
        &self,
        target: &EditorTarget,
    ) -> Access {
        if (self.chats.locks)(&target.chat) {
            return Access::ReadOnly;
        }
        Access::Editable
    }

    fn unavailable_target(
        &self,
        pane: &PaneId,
    ) -> Option<EditorTarget> {
        let panes = self.panes.borrow();
        let followed = panes.get(pane)?;
        matches!(followed.phase, PanePhase::Unavailable(_)).then(|| followed.target.clone())
    }

    fn read_waiting(&self) {
        let waiting: Vec<(PaneId, EditorTarget)> = self
            .panes
            .borrow()
            .iter()
            .filter(|(_, followed)| matches!(followed.phase, PanePhase::Waiting))
            .map(|(pane, followed)| (pane.clone(), followed.target.clone()))
            .collect();
        for (pane, target) in waiting {
            self.start_read(&pane, &target);
        }
    }

    fn start_read(
        &self,
        pane: &PaneId,
        target: &EditorTarget,
    ) {
        let Some(file) = &target.file else {
            return;
        };
        let done = answer_into(self.requests.clone(), pane.clone(), file.clone());
        if let Err(error) = self.documents.read(&target.chat, file, done) {
            tracing::debug!(%error, %pane, "a file pane waits for its chat to load");
            return;
        }
        if let Some(followed) = self.panes.borrow_mut().get_mut(pane) {
            followed.reading();
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Entity, Focusable as _, IntoElement as _, TestAppContext};
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::chat_locks::ChatLocks;
    use crate::core_panes::EDITOR;
    use crate::document_request::DocumentRequest;
    use crate::test_support::FakeDocuments;

    pub(super) const PANE: &str = "f1";
    pub(super) const PATH: &str = "src/pricing.ts";
    pub(super) const TEXT: &str = "export const price = 1\n";

    pub(super) fn layout_with(state: serde_json::Value) -> Layout {
        let mut layout = Layout::default();
        let instance = PaneInstance {
            pane_id: PaneId::new(PANE).unwrap(),
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default().with("threadId", "c1").unwrap(),
            state: PaneState::new(state).unwrap(),
        };
        let commands = [
            Command::AddTab {
                id: TabId(1),
                title: String::new(),
            },
            Command::OpenPane {
                tab: TabId(1),
                instance,
                placement: None,
            },
        ];
        for command in commands {
            layout.apply(command).unwrap();
        }
        layout
    }

    pub(super) fn panes_over(
        documents: &Rc<FakeDocuments>
    ) -> (Rc<EditorPanes>, async_channel::Receiver<ShellRequest>) {
        panes_locked(documents, Rc::new(|_| false))
    }

    pub(super) fn panes_locked(
        documents: &Rc<FakeDocuments>,
        locks: ChatLocks,
    ) -> (Rc<EditorPanes>, async_channel::Receiver<ShellRequest>) {
        let (requests, heard) = async_channel::unbounded();
        let source: Rc<dyn DocumentSource> = Rc::<FakeDocuments>::clone(documents);
        let workspaces: crate::chat_workspaces::ChatWorkspaces =
            Rc::new(|chat| (chat == "c1").then(|| "w1".to_owned()));
        let chats = ChatReads { locks, workspaces };
        let drafts = crate::test_support::memory_drafts(&documents.drafts());
        (
            Rc::new(EditorPanes::new(source, chats, drafts, requests)),
            heard,
        )
    }

    pub(super) fn landed(
        panes: &EditorPanes,
        heard: &async_channel::Receiver<ShellRequest>,
    ) -> Landing {
        let Ok(ShellRequest::Document(DocumentRequest::Read { pane, read })) = heard.try_recv()
        else {
            return Landing::Quiet;
        };
        panes.land(&pane, read)
    }

    type Case = (bool, Result<&'static str, u16>, Landing);

    #[test]
    fn a_read_shows_the_file_and_a_failed_one_refuses_only_a_fresh_open() {
        let too_large = editor::OpenFailure::of(PATH, 413, None);
        assert_eq!(too_large.title(), "File too large to open");
        let cases: [Case; 4] = [
            (true, Ok(TEXT), Landing::Shown),
            (false, Ok(TEXT), Landing::Shown),
            (true, Err(413), Landing::Refused(too_large)),
            (false, Err(404), Landing::Quiet),
        ];
        for (fresh, answer, expected) in cases {
            let documents = Rc::new(FakeDocuments::default());
            documents.loading("c1");
            match answer {
                Ok(text) => documents.holding(PATH, text),
                Err(status) => documents.refusing(PATH, status),
            }
            let (panes, heard) = panes_over(&documents);
            panes.follow(&layout_with(json!({"path": PATH, "fresh": fresh})));
            assert_eq!(
                landed(&panes, &heard),
                expected,
                "fresh {fresh}, {answer:?}"
            );
        }
    }

    #[gpui::test]
    fn a_file_pane_shows_its_document_once_its_chat_loads(cx: &mut TestAppContext) {
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        assert!(documents.reads().is_empty(), "no daemon to read from yet");
        documents.loading("c1");
        cx.update(|cx| panes.data_changed(cx));
        assert_eq!(landed(&panes, &heard), Landing::Shown);
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new(PANE).unwrap();
        cx.update(|window, cx| panes.content(&pane, window, cx));
        let shown = panes.panes.borrow();
        let Some(PanePhase::Shown(view)) = shown.get(&pane).map(|followed| &followed.phase) else {
            panic!("the document is not shown");
        };
        assert_eq!(cx.read(|cx| view.read(cx).text(cx)), TEXT);
    }

    #[test]
    fn a_restored_file_that_is_gone_says_why_and_try_again_reads_it_once_more() {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        assert_eq!(landed(&panes, &heard), Landing::Quiet, "the tab stays");
        let pane = PaneId::new(PANE).unwrap();
        documents.holding(PATH, TEXT);
        panes.retry(&pane);
        assert_eq!(documents.reads(), [PATH, PATH], "read once more");
        assert_eq!(landed(&panes, &heard), Landing::Shown);
    }

    struct Drawn(Rc<EditorPanes>);

    impl gpui::Render for Drawn {
        fn render(
            &mut self,
            window: &mut Window,
            cx: &mut gpui::Context<'_, Self>,
        ) -> impl gpui::IntoElement {
            let pane = PaneId::new(PANE).unwrap();
            self.0
                .content(&pane, window, cx)
                .unwrap_or_else(|| gpui::Empty.into_any_element())
        }
    }

    pub(super) struct Opened<'cx> {
        pub(super) panes: Rc<EditorPanes>,
        pub(super) heard: async_channel::Receiver<ShellRequest>,
        pub(super) view: Entity<editorview::EditorView>,
        pub(super) window: &'cx mut gpui::VisualTestContext,
    }

    pub(super) fn opened<'cx>(
        documents: &Rc<FakeDocuments>,
        cx: &'cx mut TestAppContext,
    ) -> Opened<'cx> {
        let (panes, heard) = panes_over(documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        landed(&panes, &heard);
        let (view, window) = shown_in(&panes, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        Opened {
            panes,
            heard,
            view,
            window,
        }
    }

    pub(super) fn typed_and_kept(
        documents: &Rc<FakeDocuments>,
        cx: &mut TestAppContext,
    ) -> Rc<EditorPanes> {
        let Opened { panes, window, .. } = opened(documents, cx);
        window.simulate_input("x");
        window.update(|_, cx| panes.keep_draft(&PaneId::new(PANE).unwrap(), cx));
        panes
    }

    pub(super) fn shown_in<'cx>(
        panes: &Rc<EditorPanes>,
        cx: &'cx mut TestAppContext,
    ) -> (
        Entity<editorview::EditorView>,
        &'cx mut gpui::VisualTestContext,
    ) {
        let drawn = Rc::clone(panes);
        let (_root, window) = cx.add_window_view(|_, _| Drawn(drawn));
        window.run_until_parked();
        let pane = PaneId::new(PANE).unwrap();
        let view = panes
            .panes
            .borrow()
            .get(&pane)
            .and_then(Followed::view)
            .cloned();
        (view.expect("the document is shown"), window)
    }

    #[gpui::test]
    fn a_file_of_a_locked_workspace_ignores_typing_and_does_not_take_the_keyboard(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_locked(&documents, Rc::new(|chat| chat == "c1"));
        panes.follow(&layout_with(json!({"path": PATH})));
        assert_eq!(landed(&panes, &heard), Landing::Shown);
        let (view, window) = shown_in(&panes, cx);
        let has_keys = window.update(|window, cx| view.focus_handle(cx).is_focused(window));
        assert!(
            !has_keys,
            "a read-only file does not take the keyboard as it opens"
        );
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        window.simulate_input("x");
        assert_eq!(
            window.read(|cx| view.read(cx).text(cx)),
            TEXT,
            "typing ignored"
        );
    }
}
