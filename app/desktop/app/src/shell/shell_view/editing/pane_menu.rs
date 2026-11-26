//! Pops the menu a pane page asked for as the desktop's own, over the page, and answers it.

use gpui::{Context, Window};
use panehost::MenuAsk;

use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn pop_pane_menu(
        &mut self,
        ask: MenuAsk,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let at = self.window_point(&ask.pane, ask.at, cx);
        let (Some(at), Some(menu)) = (at, ui::NativeMenu::installed(cx)) else {
            return ask.answer(None);
        };
        let request = ui::NativeRequest {
            entries: shell_entries(&ask.entries),
            at,
            edge: ui::MenuEdge::Start,
        };
        let chosen = menu.popup(request, window, cx);
        cx.spawn(async move |_, _| ask.answer(chosen.await))
            .detach();
    }
}

fn shell_entries(entries: &[webview::MenuEntry]) -> Vec<ui::NativeEntry> {
    entries.iter().map(shell_entry).collect()
}

fn shell_entry(entry: &webview::MenuEntry) -> ui::NativeEntry {
    match entry {
        webview::MenuEntry::Item {
            label,
            enabled,
            checked,
        } => ui::NativeEntry::Item {
            label: label.clone().into(),
            enabled: *enabled,
            checked: *checked,
        },
        webview::MenuEntry::Submenu {
            label,
            enabled,
            entries,
        } => ui::NativeEntry::Submenu {
            label: label.clone().into(),
            enabled: *enabled,
            entries: shell_entries(entries),
        },
        webview::MenuEntry::Separator => ui::NativeEntry::Separator,
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::shell::shell_view::tests::{PLACED, open_shell};
    use crate::test_support::answer_menus_with;

    fn page_entries() -> Vec<webview::MenuEntry> {
        let editor = webview::MenuEntry::Item {
            label: "Editor".into(),
            enabled: true,
            checked: true,
        };
        let open_with = webview::MenuEntry::Submenu {
            label: "Open with".into(),
            enabled: false,
            entries: vec![editor],
        };
        vec![open_with, webview::MenuEntry::Separator]
    }

    fn shell_entries() -> Vec<ui::NativeEntry> {
        let editor = ui::NativeEntry::Item {
            label: "Editor".into(),
            enabled: true,
            checked: true,
        };
        let open_with = ui::NativeEntry::Submenu {
            label: "Open with".into(),
            enabled: false,
            entries: vec![editor],
        };
        vec![open_with, ui::NativeEntry::Separator]
    }

    #[gpui::test]
    fn a_page_s_menu_pops_over_the_page_and_the_page_hears_the_choice(
        cx: &mut gpui::TestAppContext
    ) {
        let mut opened = open_shell(cx);
        let popped = answer_menus_with(cx, Some(vec![1]));
        let heard = Arc::new(Mutex::new(None));
        let sink = Arc::clone(&heard);
        let ask = MenuAsk::new(
            opened.pane.clone(),
            (page_entries(), (12.0, 34.0)),
            Box::new(move |path| *sink.lock().unwrap() = Some(path)),
        );
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
                shell.pop_pane_menu(ask, window, cx);
            });
        opened.window.run_until_parked();
        let request = popped.borrow().first().cloned().expect("the menu popped");
        assert_eq!(request.entries, shell_entries());
        let (x, y) = (f32::from(request.at.x), f32::from(request.at.y));
        assert!(
            x > 312.0 && y > 74.0,
            "page point plus the page's origin: {request:?}"
        );
        assert_eq!(*heard.lock().unwrap(), Some(Some(vec![1])));
    }

    #[gpui::test]
    fn a_menu_for_a_page_not_shown_answers_nothing_chosen(cx: &mut gpui::TestAppContext) {
        let mut opened = open_shell(cx);
        let heard = Arc::new(Mutex::new(None));
        let sink = Arc::clone(&heard);
        let ask = MenuAsk::new(
            protocol::PaneId::new("gone").unwrap(),
            (Vec::new(), (1.0, 1.0)),
            Box::new(move |path| *sink.lock().unwrap() = Some(path)),
        );
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
                shell.pop_pane_menu(ask, window, cx);
            });
        assert_eq!(*heard.lock().unwrap(), Some(None));
    }
}
