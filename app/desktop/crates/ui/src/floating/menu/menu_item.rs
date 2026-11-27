mod menu_action;
mod nested_items;

pub use menu_action::MenuAction;

use std::rc::Rc;

use gpui::SharedString;

use crate::floating::menu::menu_item::nested_items::NestedItems;

#[derive(Debug, Clone)]
#[non_exhaustive]
pub enum MenuItem {
    Action(MenuAction),
    Separator,
}

impl MenuItem {
    pub(crate) fn is_selectable(&self) -> bool {
        matches!(self, Self::Action(action) if !action.disabled)
    }

    pub(crate) fn submenu_items(&self) -> Option<Rc<[MenuItem]>> {
        match self {
            Self::Action(action) => action.submenu.as_ref().and_then(NestedItems::get),
            _ => None,
        }
    }
}

impl MenuAction {
    pub fn submenu(
        label: impl Into<SharedString>,
        items: impl Into<Rc<[MenuItem]>>,
    ) -> Self {
        Self {
            submenu: Some(NestedItems::new(items.into())),
            ..Self::new(label, |_, _| {})
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_enabled_actions_are_selectable() {
        let enabled = MenuItem::Action(MenuAction::new("Open", |_, _| {}));
        let disabled = MenuItem::Action(MenuAction::new("Open", |_, _| {}).disabled());
        assert!(enabled.is_selectable());
        assert!(!disabled.is_selectable());
        assert!(!MenuItem::Separator.is_selectable());
    }

    #[test]
    fn a_submenu_row_carries_its_items() {
        let action = MenuAction::submenu("Move to", vec![MenuItem::Separator]);
        let item = MenuItem::Action(action);
        assert_eq!(item.submenu_items().map(|items| items.len()), Some(1));
        let plain = MenuItem::Action(MenuAction::new("Plain", |_, _| {}));
        assert!(plain.submenu_items().is_none());
    }
}
