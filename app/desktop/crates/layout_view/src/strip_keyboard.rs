use gpui::{App, Entity};
use model::{Command, Group};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::strip_key::{StripKey, strip_command};

pub(crate) struct StripKeyboard {
    group: Group,
    pinned: Vec<(PaneId, String)>,
}

impl StripKeyboard {
    pub(crate) fn new(
        group: Group,
        pinned: Vec<(PaneId, String)>,
    ) -> Self {
        Self { group, pinned }
    }

    pub(crate) fn press(
        &self,
        key: &str,
        view: &Entity<LayoutView>,
        cx: &mut App,
    ) {
        let Some(pressed) = strip_command(&self.group, self.group.active(), key) else {
            return;
        };
        let event = event_of(pressed, &self.pinned);
        view.update(cx, |_, cx| cx.emit(event));
    }
}

fn event_of(
    pressed: StripKey,
    pinned: &[(PaneId, String)],
) -> LayoutEvent {
    match pressed {
        StripKey::Focus(pane) => LayoutEvent::StripFocus { pane },
        StripKey::Close(pane) => LayoutEvent::Apply(Command::ClosePane { pane }),
        StripKey::PinnedStays(pane) => {
            let name = pinned
                .iter()
                .find(|(member, _)| *member == pane)
                .map_or_else(|| pane.to_string(), |(_, label)| label.clone());
            LayoutEvent::Announce(format!("Cannot close pinned tab {name}"))
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::pane_id;

    #[test]
    fn a_key_becomes_the_event_the_shell_acts_on() {
        let pinned = [(pane_id("a"), "Notes".to_owned())];
        let cases = [
            (
                "focus",
                StripKey::Focus(pane_id("b")),
                LayoutEvent::StripFocus { pane: pane_id("b") },
            ),
            (
                "close",
                StripKey::Close(pane_id("b")),
                LayoutEvent::Apply(Command::ClosePane { pane: pane_id("b") }),
            ),
            (
                "a pinned tab stays",
                StripKey::PinnedStays(pane_id("a")),
                LayoutEvent::Announce("Cannot close pinned tab Notes".into()),
            ),
            (
                "a pinned tab without a label",
                StripKey::PinnedStays(pane_id("z")),
                LayoutEvent::Announce("Cannot close pinned tab z".into()),
            ),
        ];
        for (name, pressed, expected) in cases {
            assert_eq!(event_of(pressed, &pinned), expected, "{name}");
        }
    }
}
