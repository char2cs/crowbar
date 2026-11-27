//! The shell's context menus on macOS, drawn by `AppKit`.

use std::cell::Cell;

use objc2::MainThreadMarker;
use platform::{AppkitParent, ContextMenu, MenuRow};

use crate::backend::desktop::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::{Error, Result};
use crate::menu::{MenuEntry, MenuRequest};
use crate::view::ParentWindow;

impl MenuPopping for AppkitPlatform {
    fn popup_menu(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()> {
        let ParentWindow::AppKit(address) = parent else {
            return Err(Error::UnsupportedParent(
                "a macOS menu needs an AppKit window",
            ));
        };
        let main_thread = self.main_thread;
        let asked = Cell::new(Some((request, answer)));
        platform::on_next_turn(main_thread, move || {
            pop_up(asked.take(), address, main_thread);
        });
        Ok(())
    }
}

fn pop_up(
    asked: Option<(MenuRequest, ChosenEntry)>,
    address: usize,
    main_thread: MainThreadMarker,
) {
    let Some((request, answer)) = asked else {
        return;
    };
    let rows: Vec<MenuRow> = request.entries.iter().map(row).collect();
    let shown = AppkitParent::find(address, main_thread);
    let menu = ContextMenu::new(&rows, main_thread);
    answer(shown.and_then(|view| menu.show(&view, (request.x, request.y))));
}

fn row(entry: &MenuEntry) -> MenuRow {
    match entry {
        MenuEntry::Item {
            label,
            enabled,
            checked,
        } => MenuRow::Item {
            label: label.clone(),
            enabled: *enabled,
            checked: *checked,
        },
        MenuEntry::Submenu {
            label,
            enabled,
            entries,
        } => MenuRow::Submenu {
            label: label.clone(),
            enabled: *enabled,
            rows: entries.iter().map(row).collect(),
        },
        MenuEntry::Separator => MenuRow::Separator,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_submenu_s_entries_become_its_rows() {
        let entries = vec![MenuEntry::Separator];
        let submenu = MenuEntry::Submenu {
            label: "Move to".into(),
            enabled: false,
            entries,
        };
        let want = MenuRow::Submenu {
            label: "Move to".into(),
            enabled: false,
            rows: vec![MenuRow::Separator],
        };
        assert_eq!(row(&submenu), want);
    }
}
