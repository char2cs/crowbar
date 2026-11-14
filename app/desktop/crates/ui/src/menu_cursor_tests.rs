use super::*;
use crate::menu_action::MenuAction;

fn action(disabled: bool) -> MenuItem {
    MenuItem::Action(MenuAction::new("x", |_, _| {}).disabled(disabled))
}

fn sample() -> Vec<MenuItem> {
    vec![action(false), MenuItem::Separator, action(true), action(false), MenuItem::Label("g".into())]
}

#[test]
fn stepping_skips_separators_labels_and_disabled_rows() {
    let items = sample();
    const CASES: &[(Option<usize>, bool, Option<usize>)] = &[
        (None, true, Some(0)),
        (Some(0), true, Some(3)),
        (Some(3), true, Some(0)),
        (None, false, Some(3)),
        (Some(3), false, Some(0)),
        (Some(0), false, Some(3)),
    ];
    for &(from, forward, expected) in CASES {
        assert_eq!(step(&items, from, forward), expected, "from {from:?} forward {forward}");
    }
}

#[test]
fn nothing_selectable_yields_none() {
    assert_eq!(step(&[MenuItem::Separator], None, true), None);
    assert_eq!(step(&[], None, false), None);
}
