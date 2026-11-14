use super::*;
use crate::action::Action;

#[test]
fn every_piece_converts_into_its_variant() {
    assert!(matches!(Block::from(Section::new("S")), Block::Section(_)));
    assert!(matches!(
        Block::from(Heading::new("H", "d")),
        Block::Heading(_)
    ));
}

#[test]
fn lists_groups_and_buttons_convert_too() {
    assert!(matches!(
        Block::from(ShortcutGroup::new("G", "k")),
        Block::Shortcuts(_)
    ));
    assert!(matches!(
        Block::from(ListBlock::new("s", "none")),
        Block::List(_)
    ));
    assert!(matches!(
        Block::from(ButtonSpec::new("Go", Action::backend("go"))),
        Block::Button(_)
    ));
}
