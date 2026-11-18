use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;

const FOCUS_KIND: &str = "shell.focus";

pub(crate) fn focus_report(message: &IpcMessage) -> Option<HostEvent> {
    if !message.text.contains(FOCUS_KIND) {
        return None;
    }
    let value: serde_json::Value = serde_json::from_str(&message.text).ok()?;
    (value.get("kind")?.as_str()? == FOCUS_KIND).then(|| HostEvent::PageFocused {
        pane: message.pane.clone(),
    })
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;

    fn message(text: &str) -> IpcMessage {
        IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text: text.into(),
        }
    }

    #[test]
    fn a_page_reports_that_it_took_focus_and_nothing_else_does() {
        let cases = [
            (
                "the focus report",
                r#"{"sdk":1,"kind":"shell.focus"}"#,
                Some(HostEvent::PageFocused {
                    pane: PaneId::new("p1").unwrap(),
                }),
            ),
            (
                "another frame",
                r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#,
                None,
            ),
            ("garbage", "not json shell.focus", None),
        ];
        for (name, text, expected) in cases {
            assert_eq!(focus_report(&message(text)), expected, "{name}");
        }
    }
}
