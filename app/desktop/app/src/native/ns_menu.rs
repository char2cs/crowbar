//! Pops the shell's menus as `NSMenu`s through gpui-component's native menu, which runs the menu
//! off GPUI's call stack and dispatches the chosen row's action.
//!
//! Each row's action carries its path and a sender of the menu's answer. The menu drops its rows,
//! and with them every sender, once it closes, so a menu closed without a choice answers none.

mod chosen;

use std::rc::Rc;

use async_channel::Sender;
use chosen::Chosen;
use gpui::{App, Task, Window};
use gpui_kit::component::native_menu::NativeMenu as KitMenu;

pub(super) fn install(cx: &mut App) {
    cx.on_action(answer);
    let popup: ui::PopupMenu = Rc::new(ask);
    cx.set_global(ui::NativeMenu::new(popup));
}

fn answer(
    chosen: &Chosen,
    _: &mut App,
) {
    if chosen.answer.try_send(chosen.path.clone()).is_err() {
        tracing::debug!("a menu row was chosen after its menu answered");
    }
}

// PLATFORM: macos AppKit lines a menu's left edge up with the point it opens at, whatever `request.edge` asks.
fn ask(
    request: ui::NativeRequest,
    window: &Window,
    cx: &mut App,
) -> Task<Option<ui::MenuPath>> {
    let (sender, answer) = async_channel::bounded(1);
    let menu = kit_menu(&request.entries, &[], &sender);
    drop(sender);
    let at = request.at;
    window.defer(cx, move |window, cx| menu.show(at, window, cx));
    cx.background_executor()
        .spawn(async move { answer.recv().await.ok() })
}

fn kit_menu(
    entries: &[ui::NativeEntry],
    at: &[usize],
    answer: &Sender<ui::MenuPath>,
) -> KitMenu {
    entries
        .iter()
        .enumerate()
        .fold(KitMenu::new(), |menu, (index, entry)| {
            let path = [at, &[index]].concat();
            with_row(menu, entry, path, answer)
        })
}

fn with_row(
    menu: KitMenu,
    entry: &ui::NativeEntry,
    path: ui::MenuPath,
    answer: &Sender<ui::MenuPath>,
) -> KitMenu {
    match entry {
        ui::NativeEntry::Separator => menu.separator(),
        ui::NativeEntry::Item {
            label,
            enabled: false,
            ..
        }
        | ui::NativeEntry::Submenu {
            label,
            enabled: false,
            ..
        } => menu.menu_with_disabled(label.clone(), true, Chosen::new(path, answer)),
        ui::NativeEntry::Item { label, checked, .. } => {
            menu.menu_with_check(label.clone(), *checked, Chosen::new(path, answer))
        },
        ui::NativeEntry::Submenu { label, entries, .. } => {
            menu.submenu(label.clone(), kit_menu(entries, &path, answer))
        },
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    fn request() -> ui::NativeRequest {
        let open = ui::NativeEntry::Item {
            label: "Open".into(),
            enabled: true,
            checked: false,
        };
        ui::NativeRequest {
            entries: vec![open],
            at: gpui::point(gpui::px(4.0), gpui::px(4.0)),
            edge: ui::MenuEdge::Start,
        }
    }

    #[gpui::test]
    async fn a_chosen_row_answers_its_path_and_a_menu_that_never_opened_answers_none(
        cx: &mut TestAppContext
    ) {
        let window = cx.add_empty_window();
        let (sender, answer) = async_channel::bounded(1);
        window.update(|window, cx| {
            install(cx);
            window.dispatch_action(Chosen::new(vec![2, 0], &sender), cx);
        });
        drop(sender);
        window.run_until_parked();
        assert_eq!(answer.try_recv().ok(), Some(vec![2, 0]), "chosen");
        let closed = window.update(|window, cx| ask(request(), window, cx));
        window.run_until_parked();
        assert_eq!(closed.await, None, "dismissed");
    }
}
