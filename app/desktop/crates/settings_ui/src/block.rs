//! The building blocks of a page.

use crate::button_spec::ButtonSpec;
use crate::heading::Heading;
use crate::list_block::ListBlock;
use crate::section::Section;
use crate::shortcut_group::ShortcutGroup;

/// One vertical piece of a page.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
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
#[path = "block_tests.rs"]
mod tests;
