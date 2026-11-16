use sidebar::Mode;

use crate::core_panes::{BROWSER, CHAT, EDITOR, NEW_TAB, REVIEW, TERMINAL};

/// The sidebar mode a pane of `type_id` pulls toward; `None` for a pane that pulls nowhere.
pub(crate) fn surface_of(type_id: &str) -> Option<Mode> {
    match type_id {
        EDITOR => Some(Mode::Files),
        REVIEW => Some(Mode::Changes),
        CHAT | TERMINAL | BROWSER | NEW_TAB => Some(Mode::Chats),
        _ => None,
    }
}

/// Records that focus now sits on a `now` surface, and says which mode the sidebar should follow
/// to: only a change from a surface seen before counts, so the focus restored at startup and
/// panes that pull nowhere leave the chosen mode alone.
pub(crate) fn crossing(
    last: &mut Option<Mode>,
    now: Option<Mode>,
) -> Option<Mode> {
    let now = now?;
    let before = last.replace(now)?;
    (before != now).then_some(now)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::core_panes::CONSOLE;

    #[test]
    fn each_pane_type_names_the_surface_it_belongs_to() {
        const CASES: [(&str, Option<Mode>); 8] = [
            (EDITOR, Some(Mode::Files)),
            (REVIEW, Some(Mode::Changes)),
            (CHAT, Some(Mode::Chats)),
            (TERMINAL, Some(Mode::Chats)),
            (BROWSER, Some(Mode::Chats)),
            (NEW_TAB, Some(Mode::Chats)),
            (CONSOLE, None),
            ("ext.board", None),
        ];
        for (type_id, mode) in CASES {
            assert_eq!(surface_of(type_id), mode, "{type_id}");
        }
    }

    #[test]
    fn focus_crossing_into_another_surface_moves_the_sidebar_once() {
        let mut last = None;
        let steps = [
            (Some(Mode::Chats), None),
            (Some(Mode::Chats), None),
            (None, None),
            (Some(Mode::Files), Some(Mode::Files)),
            (Some(Mode::Files), None),
            (None, None),
            (Some(Mode::Chats), Some(Mode::Chats)),
        ];
        for (now, expected) in steps {
            assert_eq!(crossing(&mut last, now), expected, "{now:?}");
        }
    }
}
