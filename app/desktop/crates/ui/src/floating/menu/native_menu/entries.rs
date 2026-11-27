//! The menu's rows as the desktop draws them, and the action a chosen path names.

use std::rc::Rc;

use gpui::{App, Window};

use crate::floating::menu::menu_item::{MenuAction, MenuItem};
use crate::floating::menu::native_menu::native_entry::NativeEntry;

type Run = Rc<dyn Fn(&mut Window, &mut App)>;

pub(super) fn entries(items: &[MenuItem]) -> Vec<NativeEntry> {
    items.iter().map(entry).collect()
}

fn entry(item: &MenuItem) -> NativeEntry {
    match item {
        MenuItem::Action(action) => action_entry(item, action),
        MenuItem::Separator => NativeEntry::Separator,
    }
}

fn action_entry(
    item: &MenuItem,
    action: &MenuAction,
) -> NativeEntry {
    let (label, enabled) = (action.label.clone(), !action.disabled);
    match item.submenu_items() {
        Some(nested) => NativeEntry::Submenu {
            label,
            enabled,
            entries: entries(&nested),
        },
        None => NativeEntry::Item {
            label,
            enabled,
            checked: action.checked,
        },
    }
}

pub(super) fn action_at(
    items: &[MenuItem],
    path: &[usize],
) -> Option<Run> {
    let (first, rest) = path.split_first()?;
    let item = items.get(*first)?;
    if !rest.is_empty() {
        return action_at(&item.submenu_items()?, rest);
    }
    let MenuItem::Action(action) = item else {
        return None;
    };
    item.is_selectable().then(|| Rc::clone(&action.on_select))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_disabled_action_is_drawn_disabled_and_runs_nothing() {
        let off = MenuAction::new("Off", |_, _| {}).disabled();
        let items = [MenuItem::Action(off), MenuItem::Separator];
        let drawn = NativeEntry::Item {
            label: "Off".into(),
            enabled: false,
            checked: false,
        };
        assert_eq!(entries(&items), [drawn, NativeEntry::Separator]);
        for path in [vec![], vec![0], vec![1], vec![9], vec![0, 4]] {
            assert!(action_at(&items, &path).is_none(), "{path:?}");
        }
        let run = MenuItem::Action(MenuAction::new("Run", |_, _| {}));
        assert!(action_at(&[run], &[0]).is_some());
    }
}
