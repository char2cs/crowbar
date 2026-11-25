//! The row, repo and foot menus as develop's native menu, when the shell installed one.

use gpui::{Context, Pixels, Point, Window};
use sidebar::Intent;
use ui::{NativeEntry, NativeMenu, NativeRequest};

use crate::overlays::popup::check::Check;
use crate::overlays::popup::menu_entry::MenuEntry;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn pop_menu(
        &mut self,
        (entries, at): (Vec<MenuEntry>, Point<Pixels>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(native) = NativeMenu::installed(cx) else {
            return self.open_menu((entries, at), window, cx);
        };
        if entries.is_empty() {
            return;
        }
        let request = NativeRequest {
            entries: native_entries(&entries),
            at,
        };
        let chosen = native.popup(request, window, cx);
        cx.spawn_in(window, async move |this, cx| {
            let intent = intent_at(&entries, &chosen.await?)?;
            this.update_in(cx, |this, window, cx| this.apply(intent, window, cx))
                .ok()
        })
        .detach();
    }
}

fn native_entries(entries: &[MenuEntry]) -> Vec<NativeEntry> {
    entries.iter().map(native_entry).collect()
}

fn native_entry(entry: &MenuEntry) -> NativeEntry {
    match entry {
        MenuEntry::Action { label, checked, .. } => NativeEntry::Item {
            label: label.clone().into(),
            enabled: true,
            checked: *checked == Some(Check::On),
        },
        MenuEntry::Submenu { label, entries, .. } => NativeEntry::Submenu {
            label: label.clone().into(),
            enabled: true,
            entries: native_entries(entries),
        },
        MenuEntry::Separator => NativeEntry::Separator,
    }
}

fn intent_at(
    entries: &[MenuEntry],
    path: &[usize],
) -> Option<Intent> {
    let (first, rest) = path.split_first()?;
    match entries.get(*first)? {
        MenuEntry::Action { intent, .. } => rest.is_empty().then(|| intent.clone()),
        MenuEntry::Submenu { entries, .. } => intent_at(entries, rest),
        MenuEntry::Separator => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    fn keep() -> Intent {
        Intent::Keep(RowId::from("chat"))
    }

    fn entries() -> Vec<MenuEntry> {
        let rename = MenuEntry::action(
            "Rename",
            ui::IconName::PencilSimpleLine,
            Intent::StartRename(RowId::from("chat")),
        );
        let target = MenuEntry::action("main", ui::IconName::GitBranch, keep());
        let move_to = MenuEntry::Submenu {
            label: "Move to…".to_owned(),
            icon: None,
            entries: vec![target],
        };
        vec![rename, MenuEntry::Separator, move_to]
    }

    #[test]
    fn the_chosen_path_finds_the_entry_even_inside_a_submenu() {
        assert_eq!(intent_at(&entries(), &[2, 0]), Some(keep()));
        for path in [vec![], vec![1], vec![2], vec![7], vec![0, 0]] {
            assert_eq!(intent_at(&entries(), &path), None, "{path:?}");
        }
    }

    #[gpui::test]
    fn a_row_menu_asks_for_a_native_menu_and_applies_the_chosen_entry(cx: &mut TestAppContext) {
        let popped = answer_menus_with(cx, Some(vec![0]));
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let at = point(px(10.0), px(10.0));
        sidebar.update_in(cx, |this, window, cx| {
            this.pop_menu((entries(), at), window, cx);
        });
        cx.run_until_parked();
        let labels = popped
            .borrow()
            .iter()
            .map(|request| menu_labels(&request.entries))
            .collect::<Vec<_>>();
        assert_eq!(labels, [["Rename", "-", "Move to…"]]);
        let state = sidebar.read_with(cx, |this, _| (this.menu.is_none(), this.inline.is_some()));
        assert_eq!(state, (true, true), "nothing drawn; Rename started");
    }
}
