//! The entries of a menu.

use gpui::SharedString;

use crate::menu_action::MenuAction;

/// One entry of a [`crate::Menu`].
#[derive(Debug, Clone)]
#[non_exhaustive]
pub enum MenuItem {
    /// A selectable row.
    Action(MenuAction),
    /// A hairline between groups.
    Separator,
    /// A small muted heading.
    Label(SharedString),
}

impl MenuItem {
    /// Whether keyboard navigation can land on this entry.
    pub(crate) fn is_selectable(&self) -> bool {
        matches!(self, Self::Action(action) if !action.disabled)
    }
}

#[cfg(test)]
#[path = "menu_item_tests.rs"]
mod tests;
