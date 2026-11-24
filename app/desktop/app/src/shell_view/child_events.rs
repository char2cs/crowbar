use gpui::{App, Context, Entity, MouseDownEvent, Subscription, Window};
use layout::Command;
use layoutview::{LayoutEvent, LayoutView};
use sidebarview::Sidebar;
use ui::ToastKind;

use super::ShellView;
use super::built::Built;
use crate::notice_key::NoticeKey;
use crate::workspace_event::WorkspaceEvent;

const WINDOW_CANNOT_HOST: &str = "This window cannot show panes: it is not an X11 window. Restart Crowbar under X11 or XWayland.";

impl ShellView {
    pub(super) fn subscribe_children(
        built: &Built,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Vec<Subscription> {
        vec![
            cx.subscribe_in(&built.workspace, window, |this, _, event, window, cx| {
                this.on_workspace(event, window, cx);
            }),
            cx.subscribe_in(&built.tabs, window, |this, _, event, window, cx| {
                this.on_tabs(event, window, cx);
            }),
            cx.subscribe_in(&built.layout, window, |this, _, event, window, cx| {
                this.on_layout(event, window, cx);
            }),
            cx.subscribe(
                &built.sidebar,
                |this, source, event: &sidebarview::Event, cx| {
                    this.on_sidebar(source, event, cx);
                },
            ),
            cx.observe_window_activation(window, |this, window, cx| {
                this.on_window_activation(window, cx);
            }),
            cx.on_app_quit(|this, _| this.close_before_quitting()),
            cx.observe_overlay_keys(window, |this, window, cx| {
                this.return_keyboard(window, cx);
            }),
            cx.observe_in(&built.sidebar, window, |this, _, window, cx| {
                this.forward_bindings(window, cx);
            }),
            cx.observe_window_bounds(window, |this, window, cx| {
                this.forward_bindings(window, cx);
            }),
        ]
    }

    pub(super) fn on_shell_pressed(
        &mut self,
        _: &MouseDownEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.state.keep_keyboard_in_shell();
        self.sync(cx);
    }

    fn close_before_quitting(&mut self) -> impl Future<Output = ()> + use<> {
        self.state.close_web_views();
        async {}
    }

    fn on_window_activation(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !window.is_window_active() {
            self.sidebar
                .update(cx, |sidebar, cx| sidebar.cancel_drag(window, cx));
        }
        self.notices
            .update(cx, |notices, cx| notices.follow_window(window, cx));
        self.return_keyboard(window, cx);
    }

    fn return_keyboard(
        &self,
        window: &Window,
        cx: &mut App,
    ) {
        let something_else_holds_the_keys =
            self.modal_open(cx) || ui::TopLayer::holds_keys(window, cx);
        if !window.is_window_active() || something_else_holds_the_keys {
            return;
        }
        self.state.apply_keyboard();
    }

    fn on_workspace(
        &mut self,
        event: &WorkspaceEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let WorkspaceEvent::Measured(bounds) = event;
        match webview::ParentWindow::from_window(window) {
            Ok(parent) => {
                self.state.set_window_width(ui::app_window_width(window));
                self.placement = Some((parent, crate::pixels::to_bounds(*bounds)));
                self.sync(cx);
            },
            Err(error) => {
                tracing::error!(%error, "the window cannot host web views");
                self.show_notice(
                    NoticeKey::Window,
                    (ToastKind::Error, WINDOW_CANNOT_HOST.into()),
                    cx,
                );
            },
        }
    }

    fn on_tabs(
        &mut self,
        event: &viewtabs::Event,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            viewtabs::Event::Apply(commands) => self.apply_guarded(commands.clone(), cx),
            viewtabs::Event::ToggleSidebar => {
                self.sidebar
                    .update(cx, |sidebar, cx| sidebar.toggle(window, cx));
            },
            viewtabs::Event::DragOut { tab, position } => self.layout.update(cx, |layout, cx| {
                layout.tab_dragged_over(*tab, *position, cx);
            }),
            viewtabs::Event::DropOut { tab, position } => self.layout.update(cx, |layout, cx| {
                layout.tab_dropped(*tab, *position, cx);
            }),
            viewtabs::Event::DragCancel => self.layout.update(cx, LayoutView::tab_drag_cancelled),
            viewtabs::Event::DropChat { chat, at } => self.drop_chat_on_strip(chat, *at, cx),
            _ => {},
        }
    }

    fn on_layout(
        &mut self,
        event: &LayoutEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            LayoutEvent::Apply(command) => return self.apply_guarded(vec![command.clone()], cx),
            LayoutEvent::StripFocus { pane } => {
                let focus = Command::FocusPane { pane: pane.clone() };
                self.state.apply_from_strip(vec![focus]);
                self.source.native().focus(pane, window, cx);
            },
            LayoutEvent::Announce(text) => {
                return self.show_notice(NoticeKey::Command, (ToastKind::Info, text.clone()), cx);
            },
            LayoutEvent::TabPath { action, path } => self.act_on_path(*action, path, cx),
            LayoutEvent::OpenChat {
                chat,
                tab,
                placement,
            } => self.drop_chat(chat, (*tab, placement), cx),
            LayoutEvent::DropTab {
                tab,
                onto,
                placement,
            } => self.drop_view(*tab, (*onto, placement), cx),
            LayoutEvent::NewTab { onto } => return self.new_tab_in(onto, cx),
            LayoutEvent::ReviewBranch { onto } => return self.review_in(onto, cx),
            LayoutEvent::ReloadTab { pane } => return self.reload_tab(pane, cx),
            LayoutEvent::RenameTab { pane, current } => {
                return self.ask_rename((pane, current), window, cx);
            },
            _ => return,
        }
        self.refresh(cx);
    }

    fn on_sidebar(
        &mut self,
        _: Entity<Sidebar>,
        event: &sidebarview::Event,
        cx: &mut Context<'_, Self>,
    ) {
        if let sidebarview::Event::Held(rows) = event {
            return self.close_held(rows, cx);
        }
        if !matches!(event, sidebarview::Event::PreferencesChanged) {
            return;
        }
        let preferences = self.sidebar.read(cx).preferences();
        let saved = self.ports.preferences.save(&preferences);
        match save_failure(&saved) {
            None => self.clear_notice(NoticeKey::Preferences, cx),
            Some(text) => {
                tracing::warn!(%text, "saving the sidebar preferences failed");
                self.show_notice(NoticeKey::Preferences, (ToastKind::Error, text), cx);
            },
        }
        self.refresh(cx);
    }

    fn close_held(
        &mut self,
        rows: &[sidebar::RowId],
        cx: &mut Context<'_, Self>,
    ) {
        let chats: Vec<String> = rows.iter().map(|row| row.as_str().to_owned()).collect();
        self.close_chats(&chats, cx);
    }

    pub(super) fn close_chats(
        &mut self,
        chats: &[String],
        cx: &mut Context<'_, Self>,
    ) {
        let panes = crate::open_chats::panes_showing(self.state.layout(), chats);
        if panes.is_empty() {
            return;
        }
        let closing = panes.into_iter().map(|pane| Command::ClosePane { pane });
        self.apply_guarded(closing.collect(), cx);
    }
}

fn save_failure<E: std::fmt::Display>(saved: &Result<(), E>) -> Option<String> {
    saved
        .as_ref()
        .err()
        .map(|error| format!("Sidebar preferences could not be saved: {error}"))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::shell_view::tests::open_shell;

    fn is_terminal(
        layout: &layout::Layout,
        pane: &protocol::PaneId,
    ) -> bool {
        let kind = layout.pane(pane).map(|instance| instance.type_id.as_str());
        kind == Some(crate::core_panes::TERMINAL)
    }

    fn terminals(shell: &ShellView) -> Vec<protocol::PaneId> {
        let layout = shell.state.layout();
        let tab = layout
            .showing("")
            .map_or_else(Vec::new, layout::Tab::pane_ids);
        tab.into_iter()
            .filter(|pane| is_terminal(layout, pane))
            .collect()
    }

    #[gpui::test]
    fn pressing_a_terminal_s_tab_hands_that_terminal_the_keyboard(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        shell.update_in(&mut opened.window, |shell, window, cx| {
            shell.new_terminal(window, cx);
            shell.new_terminal(window, cx);
            window.focus(&shell.focus, cx);
        });
        let first = shell.update(&mut opened.window, |shell, _| terminals(shell))[0].clone();
        shell.update_in(&mut opened.window, |shell, window, cx| {
            shell.on_layout(&LayoutEvent::StripFocus { pane: first }, window, cx);
        });
        opened.window.run_until_parked();
        let on_shell = shell.update_in(&mut opened.window, |shell, window, _| {
            shell.focus.is_focused(window)
        });
        assert!(!on_shell, "the terminal took the keyboard from the strip");
    }

    #[test]
    fn a_failed_save_says_why_and_a_good_one_says_nothing() {
        let cases = [
            (Ok(()), None),
            (
                Err("disk full"),
                Some("Sidebar preferences could not be saved: disk full".to_owned()),
            ),
        ];
        for (saved, expected) in cases {
            assert_eq!(save_failure(&saved), expected);
        }
    }
}
