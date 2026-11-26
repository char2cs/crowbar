//! A selectable row of a menu.

use std::rc::Rc;

use gpui::{App, SharedString, Window};

use crate::floating::menu::menu_item::nested_items::NestedItems;
use crate::floating::menu::menu_item::tone::Tone;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Pose;

pub(crate) type Select = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// A row that runs a callback when chosen.
#[derive(Clone)]
pub struct MenuAction {
    pub(crate) label: SharedString,
    pub(crate) icon: Option<IconName>,
    pub(crate) icon_weight: IconWeight,
    pub(crate) shortcut: Option<SharedString>,
    pub(crate) checked: bool,
    pub(crate) current: Pose,
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
            current: Pose::Off,
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

    /// Fills the row at rest as the page or item now shown.
    #[must_use]
    pub fn current(mut self) -> Self {
        self.current = Pose::On;
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
