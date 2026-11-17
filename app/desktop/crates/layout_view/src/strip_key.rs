use model::{Command, Group, StripStep};
use protocol::PaneId;
pub(crate) fn strip_command(
    group: &Group,
    shown: &PaneId,
    key: &str,
) -> Option<Command> {
    let step = match key {
        "left" => StripStep::Previous,
        "right" => StripStep::Next,
        "home" => StripStep::First,
        "end" => StripStep::Last,
        "delete" | "backspace" => return close_shown(group, shown),
        _ => return None,
    };
    let pane = group.step(shown, step)?.clone();
    Some(Command::FocusPane { pane })
}

fn close_shown(
    group: &Group,
    shown: &PaneId,
) -> Option<Command> {
    (!group.is_pinned(shown)).then(|| Command::ClosePane {
        pane: shown.clone(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{pane_id, strip_layout};

    fn focus(name: &str) -> Command {
        Command::FocusPane {
            pane: pane_id(name),
        }
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
            (
                "delete",
                "b",
                "delete",
                Some(Command::ClosePane { pane: pane_id("b") }),
            ),
            (
                "backspace",
                "b",
                "backspace",
                Some(Command::ClosePane { pane: pane_id("b") }),
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
    fn a_pinned_tab_is_not_closed_by_the_keyboard() {
        let mut layout = strip_layout("b");
        layout
            .apply(Command::PinPane { pane: pane_id("b") })
            .unwrap();
        let tab = layout.tab_of(&pane_id("b")).unwrap();
        let group = tab.group_of(&pane_id("b")).unwrap();
        assert_eq!(
            strip_command(group, &pane_id("b"), "delete"),
            None,
            "pinned"
        );
    }
}
