use model::{Group, StripStep};
use protocol::PaneId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum StripKey {
    Focus(PaneId),
    Close(PaneId),
    PinnedStays(PaneId),
}

pub(crate) fn strip_command(
    group: &Group,
    shown: &PaneId,
    key: &str,
) -> Option<StripKey> {
    let step = match key {
        "left" => StripStep::Previous,
        "right" => StripStep::Next,
        "home" => StripStep::First,
        "end" => StripStep::Last,
        "delete" | "backspace" => return Some(close_shown(group, shown)),
        _ => return None,
    };
    let pane = group.step(shown, step)?.clone();
    Some(StripKey::Focus(pane))
}

fn close_shown(
    group: &Group,
    shown: &PaneId,
) -> StripKey {
    let pane = shown.clone();
    if group.is_pinned(shown) {
        return StripKey::PinnedStays(pane);
    }
    StripKey::Close(pane)
}

#[cfg(test)]
mod tests {
    use model::Command;

    use super::*;
    use crate::test_support::{pane_id, strip_layout};

    fn focus(name: &str) -> StripKey {
        StripKey::Focus(pane_id(name))
    }

    #[test]
    fn keys_walk_the_strip_and_close_the_shown_tab() {
        let cases = [
            ("left", "b", "left", Some(focus("a"))),
            ("right", "b", "right", Some(focus("c"))),
            ("home", "c", "home", Some(focus("a"))),
            ("end", "a", "end", Some(focus("c"))),
            ("left at the start", "a", "left", None),
            ("right at the end", "c", "right", None),
            ("end at the end", "c", "end", None),
            ("delete", "b", "delete", Some(StripKey::Close(pane_id("b")))),
            (
                "backspace",
                "b",
                "backspace",
                Some(StripKey::Close(pane_id("b"))),
            ),
            ("another key", "b", "x", None),
        ];
        for (name, shown, key, expected) in cases {
            let layout = strip_layout(shown);
            let tab = layout.active_tab().unwrap();
            let group = tab.group_of(&pane_id("a")).unwrap();
            let got = strip_command(group, &pane_id(shown), key);
            assert_eq!(got, expected, "{name}");
        }
    }

    #[test]
    fn a_pinned_tab_is_not_closed_by_the_keyboard_and_the_user_is_told() {
        let mut layout = strip_layout("b");
        layout
            .apply(Command::PinPane { pane: pane_id("b") })
            .unwrap();
        let tab = layout.tab_of(&pane_id("b")).unwrap();
        let group = tab.group_of(&pane_id("b")).unwrap();
        assert_eq!(
            strip_command(group, &pane_id("b"), "delete"),
            Some(StripKey::PinnedStays(pane_id("b"))),
            "pinned"
        );
    }
}
