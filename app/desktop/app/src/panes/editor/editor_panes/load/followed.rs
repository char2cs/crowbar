use std::rc::Rc;

use editorview::{Document, EditorView, Opening};
use gpui::{
    AnyElement, App, AppContext as _, Entity, Focusable as _, IntoElement as _, Subscription,
    Window,
};
use protocol::PaneId;

use crate::panes::editor::document_request::DocumentRequest;
use crate::shell::inbox::request_queue::{Requests, send};
use crate::shell::inbox::shell_request::ShellRequest;

use super::arrival::Arrival;
use super::landed::Landed;
use super::landing::Landing;
use super::pane_phase::PanePhase;
use super::retry::Retry;
use super::unavailable_panel;
use crate::panes::editor::editor_panes::editor_target::EditorTarget;
use crate::panes::editor::editor_panes::view::keyboard::Keyboard;
use crate::panes::editor::editor_panes::view::view_events::listen;

pub(crate) struct Followed {
    pub(crate) target: EditorTarget,
    pub(crate) phase: PanePhase,
    pub(crate) keyboard: Keyboard,
    access: editorview::Access,
    server: Rc<dyn editorview::LanguageServer>,
    pub(crate) told: Option<Subscription>,
}

impl Followed {
    pub(crate) fn arrive(
        target: EditorTarget,
        (keyboard, access): (Keyboard, editorview::Access),
        server: Rc<dyn editorview::LanguageServer>,
    ) -> Self {
        let phase = match target.file {
            Some(_) => PanePhase::Waiting,
            None => PanePhase::Read(Landed::default()),
        };
        Self {
            target,
            phase,
            keyboard,
            access,
            server,
            told: None,
        }
    }

    pub(crate) fn shows(
        &self,
        target: &EditorTarget,
    ) -> bool {
        self.target.chat == target.chat && self.target.file == target.file
    }

    pub(crate) fn reading(&mut self) {
        self.phase = match std::mem::replace(&mut self.phase, PanePhase::Waiting) {
            PanePhase::Unavailable(failure) => PanePhase::Retrying(failure),
            _ => PanePhase::Reading,
        };
    }

    pub(crate) fn land(
        &mut self,
        read: Result<Landed, editor::OpenFailure>,
    ) -> Landing {
        let fresh = self.target.arrival == Arrival::Fresh;
        match read {
            Ok(landed) => {
                self.phase = PanePhase::Read(landed);
                Landing::Shown
            },
            Err(failure) if fresh => Landing::Refused(failure),
            Err(failure) => {
                self.phase = PanePhase::Unavailable(failure);
                Landing::Quiet
            },
        }
    }

    pub(crate) fn set_access(
        &mut self,
        access: editorview::Access,
        cx: &mut App,
    ) {
        self.access = access;
        if let Some(view) = self.view() {
            view.update(cx, |view, cx| view.set_access(access, cx));
        }
    }

    fn take_keyboard(
        &mut self,
        window: &mut Window,
        cx: &mut App,
    ) {
        let PanePhase::Shown(view) = &self.phase else {
            return;
        };
        if self.keyboard == Keyboard::Take {
            window.focus(&view.focus_handle(cx), cx);
            self.keyboard = Keyboard::Leave;
        }
    }

    pub(crate) fn view(&self) -> Option<&Entity<EditorView>> {
        match &self.phase {
            PanePhase::Shown(view) => Some(view),
            _ => None,
        }
    }

    pub(crate) fn document(
        &self,
        cx: &App,
    ) -> Option<Entity<Document>> {
        self.view().map(|view| view.read(cx).document().clone())
    }

    pub(crate) fn materialize(
        &mut self,
        (pane, requests): (&PaneId, &Requests),
        window: &mut Window,
        cx: &mut App,
    ) {
        let PanePhase::Read(landed) = &mut self.phase else {
            return;
        };
        let landed = std::mem::take(landed);
        let (document, opening) = landed.opening(&self.target, self.access);
        let server = Some(Rc::clone(&self.server));
        let document = cx.new(|cx| Document::new(document, server, cx));
        let since = document.read(cx).save_mark();
        let view = cx.new(|cx| EditorView::new(document, opening, window, cx));
        self.told = Some(listen(&view, (pane, requests), cx));
        landed.restored(since, (pane, requests));
        self.phase = PanePhase::Shown(view);
    }

    pub(crate) const fn opening(&self) -> Opening {
        Opening {
            line: self.target.line,
            access: self.access,
        }
    }

    pub(crate) fn drawn(
        &mut self,
        (pane, requests): (&PaneId, &Requests),
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        self.materialize((pane, requests), window, cx);
        self.take_keyboard(window, cx);
        let (requests, retry) = (
            requests.clone(),
            ShellRequest::Document(DocumentRequest::Retry { pane: pane.clone() }),
        );
        let on_retry = move |_: &mut App| send(&requests, retry.clone());
        match &self.phase {
            PanePhase::Shown(view) => view.clone().into_any_element(),
            PanePhase::Unavailable(failure) => {
                unavailable_panel::panel((failure, Retry::Idle), on_retry, cx)
            },
            PanePhase::Retrying(failure) => {
                unavailable_panel::panel((failure, Retry::Running), on_retry, cx)
            },
            PanePhase::Waiting | PanePhase::Joining | PanePhase::Reading | PanePhase::Read(_) => {
                gpui::div().into_any_element()
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::DocumentPath;

    use super::*;

    fn target(file: Option<&str>) -> EditorTarget {
        EditorTarget {
            chat: "c1".into(),
            file: file.map(|path| DocumentPath::new(path).unwrap()),
            label: file.unwrap_or("untitled:Untitled-1").to_owned(),
            arrival: Arrival::Restored,
            line: None,
            preview: None,
        }
    }

    fn failure() -> editor::OpenFailure {
        editor::OpenFailure::of("a.ts", 404, None)
    }

    fn phase_name(phase: &PanePhase) -> &'static str {
        match phase {
            PanePhase::Waiting => "waiting",
            PanePhase::Joining => "joining",
            PanePhase::Reading => "reading",
            PanePhase::Read(_) => "read",
            PanePhase::Shown(_) => "shown",
            PanePhase::Unavailable(_) => "unavailable",
            PanePhase::Retrying(_) => "retrying",
        }
    }

    #[test]
    fn a_pane_keeps_saying_why_its_file_is_missing_while_a_try_again_runs() {
        let cases = [
            (PanePhase::Waiting, "reading"),
            (PanePhase::Unavailable(failure()), "retrying"),
        ];
        for (phase, expected) in cases {
            let mut followed = Followed::arrive(
                target(Some("a.ts")),
                (Keyboard::Leave, editorview::Access::Editable),
                crate::test_support::FakeServers::default().servers()("c1"),
            );
            followed.phase = phase;
            followed.reading();
            assert_eq!(phase_name(&followed.phase), expected);
        }
    }

    #[test]
    fn an_untitled_pane_has_its_empty_text_at_once_and_follows_only_its_own_file() {
        let untitled = Followed::arrive(
            target(None),
            (Keyboard::Leave, editorview::Access::Editable),
            crate::test_support::FakeServers::default().servers()("c1"),
        );
        assert_eq!(phase_name(&untitled.phase), "read");
        let workspace = Followed::arrive(
            target(Some("a.ts")),
            (Keyboard::Leave, editorview::Access::Editable),
            crate::test_support::FakeServers::default().servers()("c1"),
        );
        assert!(workspace.shows(&target(Some("a.ts"))));
        assert!(
            !workspace.shows(&target(Some("b.ts"))),
            "another file is another pane"
        );
    }

    #[gpui::test]
    fn a_shown_file_opens_its_document_at_its_chat_s_language_server(
        cx: &mut gpui::TestAppContext
    ) {
        use crate::panes::editor::editor_panes::tests::{PATH, TEXT, opened};
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(crate::test_support::FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let _opened = opened(&documents, cx);
        let told = documents.servers().told();
        let expected = serde_json::json!({"path": PATH, "languageId": "typescript", "text": TEXT});
        assert_eq!(told, [("c1".to_owned(), "didOpen", expected)]);
    }
}
