use super::*;

fn routed(keystroke: &str, mode: KeyMode) -> Option<Intent> {
    route(&Keystroke::parse(keystroke).unwrap(), mode)
}

#[test]
fn capturing_takes_every_key_including_escape() {
    assert!(matches!(
        routed("escape", KeyMode::Capturing),
        Some(Intent::Key(_))
    ));
    assert!(matches!(
        routed("a", KeyMode::Capturing),
        Some(Intent::Key(_))
    ));
}

#[test]
fn escape_closes_from_the_rail_and_from_the_page() {
    assert!(matches!(
        routed("escape", KeyMode::Rail),
        Some(Intent::Close)
    ));
    assert!(matches!(
        routed("escape", KeyMode::Page),
        Some(Intent::Close)
    ));
}

#[test]
fn arrows_move_the_rail_only_while_it_has_focus() {
    const CASES: &[(&str, RailMove)] = &[
        ("down", RailMove::Next),
        ("right", RailMove::Next),
        ("up", RailMove::Previous),
        ("left", RailMove::Previous),
        ("home", RailMove::First),
        ("end", RailMove::Last),
    ];
    for &(key, expected) in CASES {
        assert!(
            matches!(routed(key, KeyMode::Rail), Some(Intent::Move(m)) if m == expected),
            "{key}"
        );
        assert!(routed(key, KeyMode::Page).is_none(), "{key}");
    }
    assert!(routed("a", KeyMode::Rail).is_none());
}
