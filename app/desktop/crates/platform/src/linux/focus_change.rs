use x11rb::protocol::Event;
use x11rb::protocol::xproto::{NotifyDetail, NotifyMode};

const WITHIN_THE_WINDOW: [NotifyDetail; 4] = [
    NotifyDetail::INFERIOR,
    NotifyDetail::POINTER,
    NotifyDetail::POINTER_ROOT,
    NotifyDetail::NONE,
];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FocusChange {
    Gained(u32),
    Lost(u32),
}

impl FocusChange {
    pub(crate) fn of(event: &Event) -> Option<Self> {
        let (focus, change) = match event {
            Event::FocusIn(focus) => (focus, Self::Gained(focus.event)),
            Event::FocusOut(focus) => (focus, Self::Lost(focus.event)),
            _ => return None,
        };
        let stays = WITHIN_THE_WINDOW.contains(&focus.detail);
        let grabbed = matches!(focus.mode, NotifyMode::GRAB | NotifyMode::UNGRAB);
        (!stays && !grabbed).then_some(change)
    }
}

#[cfg(test)]
mod tests {
    use x11rb::protocol::xproto::{FocusInEvent, MapNotifyEvent};

    use super::*;

    const WINDOW: u32 = 7;

    fn focus_in(
        detail: NotifyDetail,
        mode: NotifyMode,
    ) -> FocusInEvent {
        FocusInEvent {
            detail,
            event: WINDOW,
            mode,
            ..FocusInEvent::default()
        }
    }

    #[test]
    fn only_a_move_into_or_out_of_the_window_is_a_change() {
        let normal = NotifyMode::NORMAL;
        let cases = [
            (
                "in from the parent",
                Event::FocusIn(focus_in(NotifyDetail::ANCESTOR, normal)),
                Some(FocusChange::Gained(WINDOW)),
            ),
            (
                "out to another client",
                Event::FocusOut(focus_in(NotifyDetail::NONLINEAR, normal)),
                Some(FocusChange::Lost(WINDOW)),
            ),
            (
                "inside the window",
                Event::FocusOut(focus_in(NotifyDetail::INFERIOR, normal)),
                None,
            ),
            (
                "a grab",
                Event::FocusOut(focus_in(NotifyDetail::NONLINEAR, NotifyMode::GRAB)),
                None,
            ),
            (
                "not focus",
                Event::MapNotify(MapNotifyEvent::default()),
                None,
            ),
        ];
        for (name, event, wanted) in cases {
            assert_eq!(FocusChange::of(&event), wanted, "{name}");
        }
    }
}
