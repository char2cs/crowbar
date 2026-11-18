//! The building blocks of a page.

use crate::button_spec::ButtonSpec;
use crate::heading::Heading;
use crate::list_block::ListBlock;
use crate::section::Section;
use crate::shortcut_group::ShortcutGroup;

/// One vertical piece of a page.
#[derive(Debug, Clone, PartialEq)]
pub enum Block {
    /// Titled rows.
    Section(Section),
    /// A large heading with an explanation.
    Heading(Heading),
    /// A category of rebindable shortcuts.
    Shortcuts(ShortcutGroup),
    /// A list of backend records.
    List(ListBlock),
    /// A button between blocks.
    Button(ButtonSpec),
}

impl From<Section> for Block {
    fn from(section: Section) -> Self {
        Self::Section(section)
    }
}

impl From<Heading> for Block {
    fn from(heading: Heading) -> Self {
        Self::Heading(heading)
    }
}

impl From<ShortcutGroup> for Block {
    fn from(group: ShortcutGroup) -> Self {
        Self::Shortcuts(group)
    }
}

impl From<ListBlock> for Block {
    fn from(list: ListBlock) -> Self {
        Self::List(list)
    }
}

impl From<ButtonSpec> for Block {
    fn from(button: ButtonSpec) -> Self {
        Self::Button(button)
    }
}

#[cfg(test)]
mod tests {
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
}
