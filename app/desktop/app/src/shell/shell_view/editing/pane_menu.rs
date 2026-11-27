use gpui::{Context, Window};
use panehost::MenuAsk;

use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn pop_pane_menu(
        &mut self,
        ask: MenuAsk,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let at = self.window_point(&ask.pane, ask.at, cx);
        let (Some(at), Some((parent, _))) = (at, self.pane_area.placement) else {
            return ask.answer(None);
        };
        let request = webview::MenuRequest {
            entries: ask.entries.clone(),
            x: f64::from(f32::from(at.x)),
            y: f64::from(f32::from(at.y)),
            edge: webview::MenuEdge::Start,
        };
        let menu = self.state.host().desktop();
        let popped = menu.popup(parent, request, Box::new(move |path| ask.answer(path)));
        if let Err(error) = popped {
            tracing::warn!(%error, "a pane's context menu could not open");
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::shell::shell_view::tests::{PLACED, open_shell};

    #[gpui::test]
    fn a_page_s_menu_pops_over_the_page_and_the_page_hears_the_choice(
        cx: &mut gpui::TestAppContext
    ) {
        let mut opened = open_shell(cx);
        opened.mock.pick_entry(vec![1]);
        let heard = Arc::new(Mutex::new(None));
        let sink = Arc::clone(&heard);
        let entries = vec![webview::MenuEntry::Separator];
        let ask = MenuAsk::new(
            opened.pane.clone(),
            (entries.clone(), (12.0, 34.0)),
            Box::new(move |path| *sink.lock().unwrap() = Some(path)),
        );
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
                shell.pop_pane_menu(ask, window, cx);
            });
        let popped = opened.mock.calls().into_iter().find_map(|call| match call {
            webview::Call::PopupMenu { request, .. } => Some(request),
            _ => None,
        });
        let request = popped.expect("the menu popped");
        assert_eq!(request.entries, entries);
        assert!(
            request.x > 312.0 && request.y > 74.0,
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
