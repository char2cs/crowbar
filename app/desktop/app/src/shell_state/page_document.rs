use panehost::HostEvent;

use crate::panes::editor::document_request::DocumentRequest;

pub(super) fn page_document(event: &HostEvent) -> Option<DocumentRequest> {
    match event {
        HostEvent::DocumentEdited { pane, text } => Some(DocumentRequest::PageEdited {
            pane: pane.clone(),
            text: text.clone(),
        }),
        HostEvent::SourceRequested { pane } => {
            Some(DocumentRequest::ShowSource { pane: pane.clone() })
        },
        HostEvent::DocumentSaveRequested { pane } => {
            Some(DocumentRequest::Save { pane: pane.clone() })
        },
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;

    #[test]
    fn a_page_s_document_frames_become_requests_of_its_file_pane() {
        let pane = PaneId::new("f1").unwrap();
        let cases = [
            (
                HostEvent::DocumentEdited {
                    pane: pane.clone(),
                    text: "# Title".into(),
                },
                Some(DocumentRequest::PageEdited {
                    pane: pane.clone(),
                    text: "# Title".into(),
                }),
            ),
            (
                HostEvent::SourceRequested { pane: pane.clone() },
                Some(DocumentRequest::ShowSource { pane: pane.clone() }),
            ),
            (
                HostEvent::DocumentSaveRequested { pane: pane.clone() },
                Some(DocumentRequest::Save { pane: pane.clone() }),
            ),
            (HostEvent::Mounted { pane }, None),
        ];
        for (event, expected) in cases {
            assert_eq!(page_document(&event), expected, "{event:?}");
        }
    }
}
