//! The pointer moving over a pane's page, which GPUI cannot see over a native web view.

use protocol::PaneId;
use serde_json::json;

use crate::slot::inbox::host_event::HostEvent;
use crate::slot::inbox::ipc_message::IpcMessage;

const POINTER_KIND: &str = "shell.pointer";

pub(crate) fn pointer_message(
    pane: PaneId,
    (x, y): (f64, f64),
) -> IpcMessage {
    let text = json!({ "sdk": 1, "kind": POINTER_KIND, "x": x, "y": y }).to_string();
    IpcMessage { pane, text }
}

pub(crate) fn pointer_report(message: &IpcMessage) -> Option<HostEvent> {
    if !message.text.contains(POINTER_KIND) {
        return None;
    }
    let value: serde_json::Value = serde_json::from_str(&message.text).ok()?;
    if value.get("kind")?.as_str()? != POINTER_KIND {
        return None;
    }
    Some(HostEvent::PointerMoved {
        pane: message.pane.clone(),
        x: value.get("x")?.as_f64()?,
        y: value.get("y")?.as_f64()?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_move_reaches_the_shell_as_the_pane_and_point() {
        let pane = PaneId::new("p1").unwrap();
        let message = pointer_message(pane.clone(), (3.5, 120.0));
        assert_eq!(
            pointer_report(&message),
            Some(HostEvent::PointerMoved {
                pane,
                x: 3.5,
                y: 120.0
            })
        );
    }

    #[test]
    fn other_frames_are_not_moves() {
        let message = IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text: r#"{"sdk":1,"kind":"shell.focus"}"#.into(),
        };
        assert_eq!(pointer_report(&message), None);
    }
}
