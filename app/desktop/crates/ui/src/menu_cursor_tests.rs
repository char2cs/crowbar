use super::*;
use crate::menu_action::MenuAction;

fn plain() -> MenuItem {
    MenuItem::Action(MenuAction::new("x", |_, _| {}))
}

fn off() -> MenuItem {
    MenuItem::Action(MenuAction::new("x", |_, _| {}).disabled())
}

fn sample() -> Vec<MenuItem> {
    vec![
        plain(),
        MenuItem::Separator,
        off(),
        plain(),
        MenuItem::Label("g".into()),
    ]
}

#[test]
fn stepping_skips_separators_labels_and_disabled_rows() {
    use Direction::{Backward, Forward};
    let items = sample();
    let cases = [
        (None, Forward, Some(0)),
        (Some(0), Forward, Some(3)),
        (Some(3), Forward, Some(0)),
        (None, Backward, Some(3)),
        (Some(3), Backward, Some(0)),
        (Some(0), Backward, Some(3)),
    ];
    for (from, direction, expected) in cases {
        assert_eq!(
            step(&items, from, direction),
            expected,
            "from {from:?} {direction:?}"
        );
    }
}

#[test]
fn nothing_selectable_yields_none() {
    assert_eq!(step(&[MenuItem::Separator], None, Direction::Forward), None);
    assert_eq!(step(&[], None, Direction::Backward), None);
}
