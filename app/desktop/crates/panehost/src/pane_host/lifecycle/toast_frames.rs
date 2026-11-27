use protocol::{PaneFrame, PaneId};

use crate::slot::HostEvent;

pub(crate) fn toast_events(
    pane: PaneId,
    frame: PaneFrame,
) -> Vec<HostEvent> {
    let event = match frame {
        PaneFrame::Toast(toast) => HostEvent::ToastShown { pane, toast },
        PaneFrame::ToastClose { key } => HostEvent::ToastClosed { pane, key },
        PaneFrame::OpenRefused { title, message } => HostEvent::OpenRefused {
            pane,
            title,
            message,
        },
        _ => return Vec::new(),
    };
    vec![event]
}

#[cfg(test)]
mod tests {
    use protocol::DisplayText;

    use super::*;

    #[test]
    fn only_a_toast_frame_becomes_a_toast_event() {
        let pane = PaneId::new("p1").unwrap();
        let key = DisplayText::new("save").unwrap();
        let close = PaneFrame::ToastClose { key: key.clone() };
        let closed = HostEvent::ToastClosed {
            pane: pane.clone(),
            key,
        };
        assert_eq!(toast_events(pane.clone(), close), vec![closed]);
        let other = PaneFrame::SetDirty { dirty: true };
        assert_eq!(toast_events(pane, other), Vec::new());
    }
}
