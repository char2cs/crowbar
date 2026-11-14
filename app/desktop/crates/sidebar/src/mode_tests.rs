use super::*;

#[test]
fn availability_follows_the_focused_workspace() {
    let cases = [
        (Mode::Chats, Focus::Nothing, true),
        (Mode::Files, Focus::Nothing, false),
        (Mode::Files, Focus::Workspace, true),
        (Mode::Changes, Focus::Workspace, false),
        (Mode::Changes, Focus::Repository, true),
    ];
    for (mode, focus, expected) in cases {
        assert_eq!(
            mode.is_available(focus),
            expected,
            "{mode:?} with {focus:?}"
        );
    }
}

#[test]
fn an_unavailable_choice_falls_back_to_chats_without_being_forgotten() {
    let chosen = Mode::Changes;
    assert_eq!(chosen.shown(Focus::Nothing), Mode::Chats, "fallback");
    assert_eq!(
        chosen.shown(Focus::Repository),
        Mode::Changes,
        "back when available"
    );
}

#[test]
fn only_changes_is_hidden_when_unavailable() {
    assert!(!Mode::Changes.is_listed(Focus::Workspace), "changes hidden");
    assert!(
        Mode::Files.is_listed(Focus::Nothing),
        "files stays listed, disabled"
    );
}

#[test]
fn indices_follow_tab_order() {
    for (index, mode) in Mode::ALL.into_iter().enumerate() {
        assert_eq!(mode.index(), index, "{mode:?}");
    }
}
