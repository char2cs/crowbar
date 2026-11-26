use layout::{PaneGroup, StripStep};
use protocol::PaneId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum StripKey {
    Focus(PaneId),
    Close(PaneId),
    PinnedStays(PaneId),
}

pub(crate) fn strip_command(
    group: &PaneGroup,
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
    group: &PaneGroup,
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
    use layout::{Command, Layout};

    use super::*;
    use crate::test_support::{pane_id, strip_layout};

    fn focus(name: &str) -> StripKey {
        StripKey::Focus(pane_id(name))
    }

    fn showing(
        shown: &str,
        pinned: &[&str],
    ) -> Layout {
        let mut layout = strip_layout(shown);
        for pane in pinned.iter().map(|name| pane_id(name)) {
            layout.apply(Command::PinPane { pane }).expect("pinned");
        }
        layout
    }

    #[test]
    fn keys_walk_the_strip_and_close_the_shown_tab_unless_it_is_pinned() {
        let close_b = Some(StripKey::Close(pane_id("b")));
        let cases: [(&str, &str, &[&str], Option<StripKey>); 11] = [
            ("b", "left", &[], Some(focus("a"))),
            ("b", "right", &[], Some(focus("c"))),
            ("c", "home", &[], Some(focus("a"))),
            ("a", "end", &[], Some(focus("c"))),
            ("a", "left", &[], None),
            ("c", "right", &[], None),
            ("c", "end", &[], None),
            ("b", "delete", &[], close_b.clone()),
            ("b", "backspace", &[], close_b),
            ("b", "x", &[], None),
            (
                "b",
                "delete",
                &["b"],
                Some(StripKey::PinnedStays(pane_id("b"))),
            ),
        ];
        for (shown, key, pinned, expected) in cases {
            let layout = showing(shown, pinned);
            let group = layout.showing("").unwrap().group_of(&pane_id("a")).unwrap();
            let got = strip_command(group, &pane_id(shown), key);
            assert_eq!(got, expected, "{key} on {shown}, pinned: {pinned:?}");
        }
    }
}
