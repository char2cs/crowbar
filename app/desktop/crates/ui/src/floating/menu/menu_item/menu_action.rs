use std::rc::Rc;

use gpui::{App, SharedString, Window};

use crate::floating::menu::menu_item::nested_items::NestedItems;

pub(crate) type Select = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

#[derive(Clone)]
pub struct MenuAction {
    pub(crate) label: SharedString,
    pub(crate) checked: bool,
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
    pub fn new(
        label: impl Into<SharedString>,
        on_select: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            label: label.into(),
            checked: false,
            disabled: false,
            submenu: None,
            on_select: Rc::new(on_select),
        }
    }

    #[must_use]
    pub fn checked(mut self) -> Self {
        self.checked = true;
        self
    }

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
    fn a_new_action_is_enabled_and_unchecked() {
        let action = action();
        assert_eq!(action.label.as_ref(), "Rename");
        assert!(!action.checked && !action.disabled);
    }

    #[test]
    fn builders_set_their_field() {
        let action = action().checked().disabled();
        assert!(action.checked && action.disabled);
    }
}
