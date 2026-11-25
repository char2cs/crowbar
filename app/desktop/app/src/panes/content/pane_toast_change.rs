use panehost::HostEvent;
use protocol::{DisplayText, PaneId, PaneToast};

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum PaneToastChange {
    Shown(PaneId, PaneToast),
    Closed(PaneId, DisplayText),
    OpenRefused(PaneId, (Option<DisplayText>, DisplayText)),
}

impl PaneToastChange {
    pub(crate) fn of(event: HostEvent) -> Option<Self> {
        match event {
            HostEvent::ToastShown { pane, toast } => Some(Self::Shown(pane, toast)),
            HostEvent::ToastClosed { pane, key } => Some(Self::Closed(pane, key)),
            HostEvent::OpenRefused {
                pane,
                title,
                message,
            } => Some(Self::OpenRefused(pane, (title, message))),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::{DisplayText, ToastTone};

    use super::*;

    #[test]
    fn only_toast_events_are_toast_changes() {
        let pane = PaneId::new("p1").unwrap();
        let key = DisplayText::new("k").unwrap();
        let toast = PaneToast {
            key: key.clone(),
            tone: ToastTone::Info,
            title: DisplayText::new("Saved").unwrap(),
            description: None,
            pinned: false,
        };
        let shown = HostEvent::ToastShown {
            pane: pane.clone(),
            toast: toast.clone(),
        };
        let closed = HostEvent::ToastClosed {
            pane: pane.clone(),
            key: key.clone(),
        };
        let hover = HostEvent::HoverChanged {
            pane: pane.clone(),
            hovered: true,
        };
        let changes = [shown, closed, hover].map(PaneToastChange::of);
        let expected = [
            Some(PaneToastChange::Shown(pane.clone(), toast)),
            Some(PaneToastChange::Closed(pane, key)),
            None,
        ];
        assert_eq!(changes, expected);
    }
}
