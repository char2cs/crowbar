use std::sync::Arc;

use serde_json::json;
use webview::{PageEvent, PageHandler};

use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;
use crate::slot::owner::Owner;

const FOCUS_KIND: &str = "shell.focus";

pub(crate) fn press_handler(
    inner: Option<PageHandler>,
    owner: Owner,
    sink: async_channel::Sender<IpcMessage>,
) -> PageHandler {
    Arc::new(move |event| on_page(inner.as_ref(), &owner, &sink, event))
}

fn on_page(
    inner: Option<&PageHandler>,
    owner: &Owner,
    sink: &async_channel::Sender<IpcMessage>,
    event: PageEvent,
) -> bool {
    if event != PageEvent::Pressed {
        return inner.is_none_or(|handler| handler(event));
    }
    let Some(pane) = owner.current() else {
        return true;
    };
    let text = json!({ "sdk": 1, "kind": FOCUS_KIND }).to_string();
    if sink.try_send(IpcMessage { pane, text }).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a press on a page");
    }
    true
}

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

    #[test]
    fn a_press_on_a_page_reports_that_it_took_focus_and_other_events_pass_through() {
        let pane = PaneId::new("p1").unwrap();
        let owner = Owner::default();
        owner.assign(pane.clone());
        let (sink, inbox) = async_channel::unbounded();
        let inner: PageHandler = Arc::new(|event| event != PageEvent::Title("veto".into()));
        let handler = press_handler(Some(inner), owner, sink);
        assert!(handler(PageEvent::Pressed), "a press is never refused");
        let reported = inbox.try_recv().ok().as_ref().and_then(focus_report);
        assert_eq!(reported, Some(HostEvent::PageFocused { pane }), "press");
        assert!(
            !handler(PageEvent::Title("veto".into())),
            "the inner answer"
        );
        assert!(inbox.try_recv().is_err(), "nothing else is reported");
    }

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
