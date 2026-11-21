use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use gpui::{AnyElement, App, Window};
use layout::{Layout, Tab};
use protocol::PaneId;

use crate::document_source::{DocumentSource, answer_into, save_failed, saved_into};
use crate::request_queue::send;
use crate::shell_request::ShellRequest;

mod arrival;
mod editor_target;
mod followed;
mod landing;
mod pane_phase;
mod retry;
mod unavailable_panel;

pub(crate) use editor_target::EditorTarget;
use followed::Followed;
pub(crate) use landing::Landing;
use pane_phase::PanePhase;

pub(crate) struct EditorPanes {
    documents: Rc<dyn DocumentSource>,
    panes: RefCell<BTreeMap<PaneId, Followed>>,
    requests: async_channel::Sender<ShellRequest>,
}

impl EditorPanes {
    pub(crate) fn new(
        documents: Rc<dyn DocumentSource>,
        requests: async_channel::Sender<ShellRequest>,
    ) -> Self {
        Self {
            documents,
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
        let mut panes = self.panes.borrow_mut();
        panes.retain(|pane, followed| open.get(pane).is_some_and(|target| followed.shows(target)));
        for (pane, target) in open {
            panes
                .entry(pane)
                .or_insert_with(|| Followed::arrive(target));
        }
        drop(panes);
        self.read_waiting();
    }

    pub(crate) fn data_changed(&self) {
        self.read_waiting();
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
        panes
            .get_mut(pane)
            .map_or(Landing::Quiet, |followed| followed.land(read))
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

    pub(crate) fn save(
        &self,
        pane: &PaneId,
        cx: &App,
    ) {
        let panes = self.panes.borrow();
        let Some(followed) = panes.get(pane) else {
            return;
        };
        let (Some(view), Some(file)) = (followed.view(), &followed.target.file) else {
            return;
        };
        let text = view.read(cx).text(cx);
        let done = saved_into(self.requests.clone(), pane.clone(), text.clone());
        let target = (followed.target.chat.as_str(), file);
        if let Err(error) = self.documents.write(target, &text, done) {
            send(&self.requests, save_failed(pane.clone(), &error));
        }
    }

    pub(crate) fn saved(
        &self,
        pane: &PaneId,
        text: &str,
        cx: &mut App,
    ) {
        let view = self
            .panes
            .borrow()
            .get(pane)
            .and_then(Followed::view)
            .cloned();
        if let Some(view) = view {
            view.update(cx, |view, cx| view.mark_saved(text, cx));
        }
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
    use gpui::TestAppContext;
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::core_panes::EDITOR;
    use crate::document_request::DocumentRequest;
    use crate::test_support::FakeDocuments;

    const PANE: &str = "f1";
    const PATH: &str = "src/pricing.ts";
    const TEXT: &str = "export const price = 1\n";

    fn layout_with(state: serde_json::Value) -> Layout {
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

    fn panes_over(
        documents: &Rc<FakeDocuments>
    ) -> (EditorPanes, async_channel::Receiver<ShellRequest>) {
        let (requests, heard) = async_channel::unbounded();
        let source: Rc<dyn DocumentSource> = Rc::<FakeDocuments>::clone(documents);
        (EditorPanes::new(source, requests), heard)
    }

    fn landed(
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
        panes.data_changed();
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

    #[gpui::test]
    fn a_save_writes_the_shown_text_and_tells_the_shell_it_reached_the_file(
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
        let (_root, window) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new(PANE).unwrap();
        window.update(|window, cx| panes.content(&pane, window, cx));
        window.update(|_, cx| panes.save(&pane, cx));
        assert_eq!(documents.writes(), [(PATH.to_owned(), TEXT.to_owned())]);
        let saved = ShellRequest::Document(DocumentRequest::Saved {
            pane,
            text: TEXT.to_owned(),
        });
        assert_eq!(heard.try_recv().unwrap(), saved);
    }
}
