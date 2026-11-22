use editorview::{EditorEvent, EditorView};
use gpui::{
    AnyElement, App, AppContext as _, Entity, Focusable as _, IntoElement as _, Subscription,
    Window,
};
use protocol::PaneId;

use crate::document_request::DocumentRequest;
use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

use super::arrival::Arrival;
use super::editor_target::EditorTarget;
use super::keyboard::Keyboard;
use super::landing::Landing;
use super::pane_phase::PanePhase;
use super::retry::Retry;
use super::unavailable_panel;

pub(super) struct Followed {
    pub(super) target: EditorTarget,
    pub(super) phase: PanePhase,
    pub(super) keyboard: Keyboard,
    access: editorview::Access,
    pub(super) told: Option<Subscription>,
}

impl Followed {
    pub(super) fn arrive(
        target: EditorTarget,
        (keyboard, access): (Keyboard, editorview::Access),
    ) -> Self {
        let phase = match target.file {
            Some(_) => PanePhase::Waiting,
            None => PanePhase::Read(String::new()),
        };
        Self {
            target,
            phase,
            keyboard,
            access,
            told: None,
        }
    }

    pub(super) fn shows(
        &self,
        target: &EditorTarget,
    ) -> bool {
        self.target.chat == target.chat && self.target.file == target.file
    }

    pub(super) fn reading(&mut self) {
        self.phase = match std::mem::replace(&mut self.phase, PanePhase::Waiting) {
            PanePhase::Unavailable(failure) => PanePhase::Retrying(failure),
            _ => PanePhase::Reading,
        };
    }

    pub(super) fn land(
        &mut self,
        read: Result<String, editor::OpenFailure>,
    ) -> Landing {
        let fresh = self.target.arrival == Arrival::Fresh;
        match read {
            Ok(text) => {
                self.phase = PanePhase::Read(text);
                Landing::Shown
            },
            Err(failure) if fresh => Landing::Refused(failure),
            Err(failure) => {
                self.phase = PanePhase::Unavailable(failure);
                Landing::Quiet
            },
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

    pub(super) fn view(&self) -> Option<&Entity<EditorView>> {
        match &self.phase {
            PanePhase::Shown(view) => Some(view),
            _ => None,
        }
    }

    pub(super) fn drawn(
        &mut self,
        (pane, requests): (&PaneId, &Requests),
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        if let PanePhase::Read(text) = &mut self.phase {
            let opening = opening(&self.target, std::mem::take(text), self.access);
            let view = cx.new(|cx| EditorView::new(opening, window, cx));
            self.told = Some(listen(&view, (pane, requests), cx));
            self.phase = PanePhase::Shown(view);
        }
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
            PanePhase::Waiting | PanePhase::Reading | PanePhase::Read(_) => {
                gpui::div().into_any_element()
            },
        }
    }
}

pub(super) fn listen(
    view: &Entity<EditorView>,
    (pane, requests): (&PaneId, &Requests),
    cx: &mut App,
) -> Subscription {
    let tell = teller(pane, requests);
    cx.subscribe(view, move |_, event: &EditorEvent, _| tell(*event))
}

fn teller(
    pane: &PaneId,
    requests: &Requests,
) -> impl Fn(EditorEvent) + 'static {
    let (requests, pane) = (requests.clone(), pane.clone());
    move |event| send(&requests, told(&pane, event))
}

fn told(
    pane: &PaneId,
    event: EditorEvent,
) -> ShellRequest {
    let pane = pane.clone();
    match event {
        EditorEvent::DirtyChanged(dirty) => {
            ShellRequest::Document(DocumentRequest::Dirty { pane, dirty })
        },
        EditorEvent::SaveRequested => ShellRequest::Document(DocumentRequest::Save { pane }),
        EditorEvent::EditsPaused => ShellRequest::Document(DocumentRequest::Paused { pane }),
    }
}

fn opening(
    target: &EditorTarget,
    text: String,
    access: editorview::Access,
) -> editorview::Opening {
    editorview::Opening {
        text,
        path: target.file.as_ref().map(|file| file.as_str().to_owned()),
        line: target.line,
        access,
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
            arrival: Arrival::Restored,
            line: None,
        }
    }

    fn failure() -> editor::OpenFailure {
        editor::OpenFailure::of("a.ts", 404, None)
    }

    fn phase_name(phase: &PanePhase) -> &'static str {
        match phase {
            PanePhase::Waiting => "waiting",
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
        );
        assert_eq!(phase_name(&untitled.phase), "read");
        let workspace = Followed::arrive(
            target(Some("a.ts")),
            (Keyboard::Leave, editorview::Access::Editable),
        );
        assert!(workspace.shows(&target(Some("a.ts"))));
        assert!(
            !workspace.shows(&target(Some("b.ts"))),
            "another file is another pane"
        );
    }
}
