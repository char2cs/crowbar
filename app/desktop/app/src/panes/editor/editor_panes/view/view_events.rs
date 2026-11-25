//! What a pane's view tells the shell: each view event as the request the shell acts on.

use editorview::{EditorEvent, EditorView};
use gpui::{App, Entity, Subscription};
use protocol::PaneId;

use crate::panes::editor::document_request::DocumentRequest;
use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

pub(crate) fn listen(
    view: &Entity<EditorView>,
    (pane, requests): (&PaneId, &Requests),
    cx: &mut App,
) -> Subscription {
    let tell = teller(pane, requests);
    cx.subscribe(view, move |_, event: &EditorEvent, _| tell(event.clone()))
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
        EditorEvent::PreviewRequested => ShellRequest::Document(DocumentRequest::Preview { pane }),
        EditorEvent::RichViewRequested => {
            ShellRequest::Document(DocumentRequest::ShowRich { pane })
        },
        EditorEvent::Definition {
            path,
            line,
            character,
        } => ShellRequest::Document(DocumentRequest::Definition {
            pane,
            path,
            at: (line, character),
        }),
        EditorEvent::DefinitionOutside(path) => {
            ShellRequest::Document(DocumentRequest::DefinitionOutside { pane, path })
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_view_s_events_reach_the_shell_as_requests_about_its_pane() {
        let pane = PaneId::new("f1").unwrap();
        let cases = [
            (
                EditorEvent::DirtyChanged(true),
                DocumentRequest::Dirty {
                    pane: pane.clone(),
                    dirty: true,
                },
            ),
            (
                EditorEvent::EditsPaused,
                DocumentRequest::Paused { pane: pane.clone() },
            ),
            (
                EditorEvent::Definition {
                    path: "src/a.ts".to_owned(),
                    line: 2,
                    character: 4,
                },
                DocumentRequest::Definition {
                    pane: pane.clone(),
                    path: "src/a.ts".to_owned(),
                    at: (2, 4),
                },
            ),
        ];
        for (event, expected) in cases {
            assert_eq!(told(&pane, event), ShellRequest::Document(expected));
        }
    }
}
