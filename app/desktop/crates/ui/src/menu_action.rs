//! A selectable row of a menu.

use std::rc::Rc;

use gpui::{App, SharedString, Window};

use crate::icon_name::IconName;

pub(crate) type Select = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// A row that runs a callback when chosen.
#[derive(Clone)]
pub struct MenuAction {
    pub(crate) label: SharedString,
    pub(crate) icon: Option<IconName>,
    pub(crate) shortcut: Option<SharedString>,
    pub(crate) checked: bool,
    pub(crate) destructive: bool,
    pub(crate) disabled: bool,
    pub(crate) on_select: Select,
}

impl std::fmt::Debug for MenuAction {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.debug_struct("MenuAction").field("label", &self.label).finish_non_exhaustive()
    }
}

impl MenuAction {
    /// A row labelled `label` that calls `on_select`.
    pub fn new(label: impl Into<SharedString>, on_select: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        Self {
            label: label.into(),
            icon: None,
            shortcut: None,
            checked: false,
            destructive: false,
            disabled: false,
            on_select: Rc::new(on_select),
        }
    }

    /// Shows a leading icon.
    #[must_use]
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Shows a shortcut hint at the end of the row.
    #[must_use]
    pub fn shortcut(mut self, keys: impl Into<SharedString>) -> Self {
        self.shortcut = Some(keys.into());
        self
    }

    /// Marks the row as the current choice with a check mark.
    #[must_use]
    pub fn checked(mut self, checked: bool) -> Self {
        self.checked = checked;
        self
    }

    /// Colours the row as destructive.
    #[must_use]
    pub fn destructive(mut self, destructive: bool) -> Self {
        self.destructive = destructive;
        self
    }

    /// Dims the row and ignores it.
    #[must_use]
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }
}
