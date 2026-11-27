use std::sync::Arc;

use protocol::PaneId;
use serde_json::json;
use webview::{PageEvent, PageHandler};

use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;
use crate::slot::inbox::pointer_message::pointer_message;
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
    match event {
        PageEvent::Pressed => report(owner, sink, focus_message),
        PageEvent::PointerMoved { x, y } => {
            report(owner, sink, |pane| pointer_message(pane, (x, y)))
        },
        event => inner.is_none_or(|handler| handler(event)),
    }
}

fn focus_message(pane: PaneId) -> IpcMessage {
    let text = json!({ "sdk": 1, "kind": FOCUS_KIND }).to_string();
    IpcMessage { pane, text }
}

fn report(
    owner: &Owner,
    sink: &async_channel::Sender<IpcMessage>,
    message: impl FnOnce(PaneId) -> IpcMessage,
) -> bool {
    let Some(pane) = owner.current() else {
        return true;
    };
    if sink.try_send(message(pane)).is_err() {
        tracing::debug!("pane host inbox is full or closed; dropping a page report");
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
    use super::*;
    use crate::slot::inbox::pointer_message::pointer_report;

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
        assert!(
            handler(PageEvent::PointerMoved { x: 2.0, y: 90.0 }),
            "a move"
        );
        let moved = inbox.try_recv().ok().as_ref().and_then(pointer_report);
        let pane = PaneId::new("p1").unwrap();
        let expected = HostEvent::PointerMoved {
            pane,
            x: 2.0,
            y: 90.0,
        };
        assert_eq!(moved, Some(expected), "move");
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
