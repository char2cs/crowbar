//! The building blocks of a page.

mod heading;
mod list_block;
mod section;

pub use heading::Heading;
pub use list_block::ListBlock;
pub use section::Section;

use crate::keys::ShortcutGroup;
use crate::schema::button_spec::ButtonSpec;

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
    use crate::schema::button_spec::Action;

    #[test]
    fn every_piece_converts_into_its_variant() {
        let blocks = [
            Block::from(Section::new("S")),
            Block::from(Heading::new("H", "d")),
            Block::from(ShortcutGroup::new("G", "k")),
            Block::from(ListBlock::new("s", "none")),
            Block::from(ButtonSpec::new("Go", Action::backend("go"))),
        ];
        let matched = blocks.iter().enumerate().all(|(index, block)| {
            let variant = match block {
                Block::Section(_) => 0,
                Block::Heading(_) => 1,
                Block::Shortcuts(_) => 2,
                Block::List(_) => 3,
                Block::Button(_) => 4,
            };
            variant == index
        });
        assert!(matched, "{blocks:?}");
    }
}
