//! A selectable row of a menu.

use std::rc::Rc;

use gpui::{App, SharedString, Window};

use crate::icon_name::IconName;
use crate::menu_tone::MenuTone;
use crate::nested_items::NestedItems;

pub(crate) type Select = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// A row that runs a callback when chosen.
#[derive(Clone)]
pub struct MenuAction {
    pub(crate) label: SharedString,
    pub(crate) icon: Option<IconName>,
    pub(crate) shortcut: Option<SharedString>,
    pub(crate) checked: bool,
    pub(crate) tone: MenuTone,
    pub(crate) disabled: bool,
    pub(crate) submenu: Option<NestedItems>,
    pub(crate) on_select: Select,
}

impl std::fmt::Debug for MenuAction {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("MenuAction")
            .field("label", &self.label)
            .finish_non_exhaustive()
    }
}

impl MenuAction {
    /// A row labelled `label` that calls `on_select`.
    pub fn new(
        label: impl Into<SharedString>,
        on_select: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            label: label.into(),
            icon: None,
            shortcut: None,
            checked: false,
            tone: MenuTone::Normal,
            disabled: false,
            submenu: None,
            on_select: Rc::new(on_select),
        }
    }

    /// Shows a leading icon.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Shows a shortcut hint at the end of the row.
    #[must_use]
    pub fn shortcut(
        mut self,
        keys: impl Into<SharedString>,
    ) -> Self {
        self.shortcut = Some(keys.into());
        self
    }

    /// Marks the row as the current choice with a check mark.
    #[must_use]
    pub fn checked(mut self) -> Self {
        self.checked = true;
        self
    }

    /// Colours the row as destructive.
    #[must_use]
    pub fn destructive(mut self) -> Self {
        self.tone = MenuTone::Destructive;
        self
    }

    /// Dims the row and ignores it.
    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn action() -> MenuAction {
        MenuAction::new("Rename", |_, _| {})
    }

    #[test]
    fn a_new_action_is_plain() {
        let action = action();
        assert_eq!(action.label.as_ref(), "Rename");
        assert!(!action.checked && !action.disabled);
        assert_eq!(action.tone, MenuTone::Normal);
        assert!(action.icon.is_none() && action.shortcut.is_none());
    }

    #[test]
    fn builders_set_their_field() {
        let action = action()
            .icon(IconName::Check)
            .shortcut("F2")
            .checked()
            .disabled()
            .destructive();
        assert_eq!(action.icon, Some(IconName::Check));
        assert_eq!(action.shortcut.as_ref().map(AsRef::as_ref), Some("F2"));
        assert!(action.checked && action.disabled);
        assert_eq!(action.tone, MenuTone::Destructive);
    }
}
