//! Pops the desktop's context menu for the shell, through the web views' platform.

use std::rc::Rc;

use gpui::{App, Task, Window};

pub(super) fn install(
    menu: webview::Desktop,
    cx: &mut App,
) {
    let popup: ui::PopupMenu =
        Rc::new(move |request, window, cx| ask(&menu, &request, (window, cx)));
    cx.set_global(ui::NativeMenu::new(popup));
}

fn ask(
    menu: &webview::Desktop,
    request: &ui::NativeRequest,
    (window, cx): (&Window, &mut App),
) -> Task<Option<ui::MenuPath>> {
    let (sender, answer) = async_channel::bounded(1);
    let send: webview::ChosenEntry = Box::new(move |path| {
        sender.try_send(path).unwrap_or_default();
    });
    let asked = webview::ParentWindow::from_window(window)
        .and_then(|parent| menu.popup(parent, web_request(request), send));
    if let Err(error) = asked {
        tracing::warn!(%error, "the context menu could not open");
    }
    cx.background_executor()
        .spawn(async move { answer.recv().await.unwrap_or_default() })
}

fn web_request(request: &ui::NativeRequest) -> webview::MenuRequest {
    webview::MenuRequest {
        entries: web_entries(&request.entries),
        x: f64::from(f32::from(request.at.x)),
        y: f64::from(f32::from(request.at.y)),
        edge: web_edge(request.edge),
    }
}

fn web_edge(edge: ui::MenuEdge) -> webview::MenuEdge {
    match edge {
        ui::MenuEdge::Start => webview::MenuEdge::Start,
        ui::MenuEdge::End => webview::MenuEdge::End,
    }
}

fn web_entries(entries: &[ui::NativeEntry]) -> Vec<webview::MenuEntry> {
    entries.iter().map(web_entry).collect()
}

fn web_entry(entry: &ui::NativeEntry) -> webview::MenuEntry {
    match entry {
        ui::NativeEntry::Item {
            label,
            enabled,
            checked,
        } => webview::MenuEntry::Item {
            label: label.to_string(),
            enabled: *enabled,
            checked: *checked,
        },
        ui::NativeEntry::Submenu {
            label,
            enabled,
            entries,
        } => webview::MenuEntry::Submenu {
            label: label.to_string(),
            enabled: *enabled,
            entries: web_entries(entries),
        },
        ui::NativeEntry::Separator => webview::MenuEntry::Separator,
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px};

    use super::*;

    fn shell_entries() -> Vec<ui::NativeEntry> {
        let open = ui::NativeEntry::Item {
            label: "Open".into(),
            enabled: true,
            checked: false,
        };
        let move_to = ui::NativeEntry::Submenu {
            label: "Move to…".into(),
            enabled: false,
            entries: vec![],
        };
        vec![open, ui::NativeEntry::Separator, move_to]
    }

    fn web_view_entries() -> Vec<webview::MenuEntry> {
        let open = webview::MenuEntry::Item {
            label: "Open".into(),
            enabled: true,
            checked: false,
        };
        let move_to = webview::MenuEntry::Submenu {
            label: "Move to…".into(),
            enabled: false,
            entries: vec![],
        };
        vec![open, webview::MenuEntry::Separator, move_to]
    }

    #[test]
    fn the_shell_s_menu_reaches_the_web_view_menu_unchanged() {
        let asked = ui::NativeRequest {
            entries: shell_entries(),
            at: point(px(12.0), px(40.5)),
            edge: ui::MenuEdge::End,
        };
        let opened = webview::MenuRequest {
            entries: web_view_entries(),
            x: 12.0,
            y: 40.5,
            edge: webview::MenuEdge::End,
        };
        assert_eq!(web_request(&asked), opened);
    }
}
