use gpui::{Context, Entity, Subscription, Window};
use layout::Command;
use layoutview::LayoutEvent;
use sidebarview::{Sidebar, SidebarEvent};
use startup::StartupEvent;
use ui::ToastKind;
use viewtabs::{TabBar, TabBarEvent};

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
            cx.subscribe(&built.tabs, Self::on_tabs),
            cx.subscribe_in(&built.layout, window, |this, _, event, window, cx| {
                this.on_layout(event, window, cx);
            }),
            cx.subscribe(&built.sidebar, |this, source, event: &SidebarEvent, cx| {
                this.on_sidebar(source, *event, cx);
            }),
            cx.subscribe_in(&built.startup, window, |this, _, event, window, cx| {
                this.on_startup(*event, window, cx);
            }),
        ]
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
        _: Entity<TabBar>,
        event: &TabBarEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TabBarEvent::Apply(commands) => self.apply_guarded(commands.clone(), cx),
            TabBarEvent::ToggleSidebar => self.sidebar.update(cx, Sidebar::toggle),
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
            },
            LayoutEvent::Announce(text) => {
                return self.show_notice(NoticeKey::Command, (ToastKind::Info, text.clone()), cx);
            },
            LayoutEvent::TabPath { action, path } => self.act_on_path(*action, path, cx),
            LayoutEvent::OpenChat {
                chat,
                tab,
                placement,
            } => self.state.open_chat_at(chat, (*tab, placement.clone())),
            LayoutEvent::NewTab { onto } => return self.new_tab_in(onto, cx),
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
        event: SidebarEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if !matches!(event, SidebarEvent::PreferencesChanged) {
            return;
        }
        let preferences = self.sidebar.read(cx).preferences();
        let saved = preferences.save(&self.preferences_path);
        match save_failure(&saved) {
            None => self.clear_notice(NoticeKey::Preferences, cx),
            Some(text) => {
                tracing::warn!(%text, "saving the sidebar preferences failed");
                self.show_notice(NoticeKey::Preferences, (ToastKind::Error, text), cx);
            },
        }
        self.place_notices(cx);
        self.refresh(cx);
    }

    fn on_startup(
        &mut self,
        event: StartupEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !matches!(event, StartupEvent::HandedOver) {
            return;
        }
        window.set_window_title(&self.window_title);
        self.startup = None;
        self.update_overlay(cx);
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
    use super::*;

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
