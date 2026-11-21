//! A selectable row of a menu.

use std::rc::Rc;

use gpui::{App, SharedString, Window};

use crate::floating::menu::menu_item::nested_items::NestedItems;
use crate::floating::menu::menu_item::tone::Tone;
use crate::foundation::IconName;
use crate::foundation::IconWeight;

pub(crate) type Select = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// A row that runs a callback when chosen.
#[derive(Clone)]
pub struct MenuAction {
    pub(crate) label: SharedString,
    pub(crate) icon: Option<IconName>,
    pub(crate) icon_weight: IconWeight,
    pub(crate) shortcut: Option<SharedString>,
    pub(crate) checked: bool,
    pub(crate) tone: Tone,
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
            icon_weight: IconWeight::Regular,
            shortcut: None,
            checked: false,
            tone: Tone::Normal,
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

    /// Draws the leading icon at `weight`, as develop's duotone page icons.
    #[must_use]
    pub fn icon_weight(
        mut self,
        weight: IconWeight,
    ) -> Self {
        self.icon_weight = weight;
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
        self.tone = Tone::Destructive;
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
        assert_eq!(action.tone, Tone::Normal);
        assert!(action.icon.is_none() && action.shortcut.is_none());
        assert_eq!(action.icon_weight, IconWeight::Regular);
    }

    #[test]
    fn builders_set_their_field() {
        let action = action()
            .icon(IconName::Check)
            .shortcut("F2")
            .checked()
            .icon_weight(IconWeight::Duotone)
            .disabled()
            .destructive();
        assert_eq!(action.icon, Some(IconName::Check));
        assert_eq!(action.icon_weight, IconWeight::Duotone);
        assert_eq!(action.shortcut.as_ref().map(AsRef::as_ref), Some("F2"));
        assert!(action.checked && action.disabled);
        assert_eq!(action.tone, Tone::Destructive);
    }
}
