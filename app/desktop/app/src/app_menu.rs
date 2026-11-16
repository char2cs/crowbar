use gpui::{App, KeyBinding, Menu, MenuItem, OsAction, SystemMenuType, actions};

actions!(crowbar_menu, [Quit, Cut, Copy, Paste]);

const APP_MENU: &str = "Crowbar";
const EDIT_MENU: &str = "Edit";
const QUIT_CHORD: &str = "cmd-q";

// PLATFORM: macos only draws the menu bar; its Edit menu routes Cut, Copy and Paste to a web view.
pub(crate) fn install(cx: &mut App) {
    cx.on_action(|_: &Quit, cx| cx.quit());
    cx.bind_keys([KeyBinding::new(QUIT_CHORD, Quit, None)]);
    cx.set_menus(menus());
}

fn menus() -> Vec<Menu> {
    vec![
        Menu {
            name: APP_MENU.into(),
            items: vec![
                MenuItem::os_submenu("Services", SystemMenuType::Services),
                MenuItem::separator(),
                MenuItem::action("Quit Crowbar", Quit),
            ],
        },
        Menu {
            name: EDIT_MENU.into(),
            items: vec![
                MenuItem::os_action("Cut", Cut, OsAction::Cut),
                MenuItem::os_action("Copy", Copy, OsAction::Copy),
                MenuItem::os_action("Paste", Paste, OsAction::Paste),
            ],
        },
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_menu_bar_has_the_app_and_edit_menus() {
        let names: Vec<_> = menus().iter().map(|menu| menu.name.to_string()).collect();
        assert_eq!(names, vec!["Crowbar", "Edit"]);
    }

    fn has_os_action(item: &MenuItem) -> bool {
        matches!(
            item,
            MenuItem::Action {
                os_action: Some(_),
                ..
            }
        )
    }

    #[test]
    fn the_edit_menu_carries_the_clipboard_os_actions() {
        let edit = menus().remove(1);
        let os_actions = edit.items.iter().filter(|item| has_os_action(item)).count();
        assert_eq!(os_actions, 3);
    }
}
