//! Which entry of an open menu is highlighted and which submenu is showing.

use crate::intent::Intent;
use crate::menu_entry::MenuEntry;
use crate::menu_step::MenuStep;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) struct MenuCursor {
    highlighted: Option<usize>,
}

impl MenuCursor {
    pub(crate) fn highlighted(self) -> Option<usize> {
        self.highlighted
    }

    pub(crate) fn highlight(&mut self, index: Option<usize>) {
        self.highlighted = index;
    }

    pub(crate) fn open_submenu(self, entries: &[MenuEntry]) -> Option<usize> {
        let index = self.highlighted?;
        matches!(entries.get(index), Some(MenuEntry::Submenu { .. })).then_some(index)
    }

    pub(crate) fn step(&mut self, step: MenuStep, entries: &[MenuEntry]) {
        let selectable = |index: &usize| !matches!(entries.get(*index), Some(MenuEntry::Separator));
        let candidates: Vec<usize> = (0..entries.len()).filter(selectable).collect();
        let next = match (step, self.highlighted) {
            (MenuStep::Next, None) => candidates.first().copied(),
            (MenuStep::Previous, None) => candidates.last().copied(),
            (MenuStep::Next, Some(from)) => candidates.iter().copied().find(|index| *index > from),
            (MenuStep::Previous, Some(from)) => {
                candidates.iter().rev().copied().find(|index| *index < from)
            },
        };
        self.highlighted = next.or(self.highlighted);
    }

    pub(crate) fn chosen(self, entries: &[MenuEntry]) -> Option<Intent> {
        match entries.get(self.highlighted?)? {
            MenuEntry::Action { intent, .. } => Some(intent.clone()),
            MenuEntry::Separator | MenuEntry::Submenu { .. } => None,
        }
    }
}

#[cfg(test)]
#[path = "menu_cursor_tests.rs"]
mod tests;
