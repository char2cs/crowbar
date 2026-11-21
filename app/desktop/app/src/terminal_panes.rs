use std::cell::RefCell;
use std::collections::BTreeMap;

use gpui::{App, AppContext as _, Entity, Focusable as _, Window};
use protocol::PaneId;
use ptysession::{Connection, Ends, TerminalSessions};
use terminalview::TerminalView;

use crate::chat_hosts::ChatHosts;
use crate::shell_request::ShellRequest;
use crate::terminal_ids::TerminalIds;
use crate::terminal_owner::TerminalOwner;

mod wiring;

pub(crate) struct TerminalPanes {
    ids: TerminalIds,
    sessions: TerminalSessions,
    hosts: ChatHosts,
    views: RefCell<BTreeMap<PaneId, Entity<TerminalView>>>,
    settings: RefCell<terminalview::Settings>,
    requests: async_channel::Sender<ShellRequest>,
}

impl TerminalPanes {
    pub(crate) fn new(
        ids: TerminalIds,
        (sessions, hosts): (TerminalSessions, ChatHosts),
        requests: async_channel::Sender<ShellRequest>,
    ) -> Self {
        Self {
            ids,
            sessions,
            hosts,
            views: RefCell::default(),
            settings: RefCell::default(),
            requests,
        }
    }

    pub(crate) fn apply(
        &self,
        settings: terminalview::Settings,
        cx: &mut App,
    ) {
        let views: Vec<_> = self.views.borrow().values().cloned().collect();
        for view in views {
            view.update(cx, |view, cx| view.set_settings(settings.clone(), cx));
        }
        *self.settings.borrow_mut() = settings;
    }

    pub(crate) fn is_terminal(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.ids.contains(pane)
    }

    pub(crate) fn focus(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(view) = self.view(pane, window, cx) {
            window.focus(&view.focus_handle(cx), cx);
        }
    }

    pub(crate) fn view(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Entity<TerminalView>> {
        self.views
            .borrow_mut()
            .retain(|open, _| self.ids.contains(open));
        if !self.ids.contains(pane) {
            return None;
        }
        let owner = self.ids.owner_of(pane);
        let existing = self.views.borrow().get(pane).cloned();
        Some(existing.unwrap_or_else(|| self.create((pane, owner.as_ref()), window, cx)))
    }

    fn create(
        &self,
        (pane, owner): (&PaneId, Option<&TerminalOwner>),
        window: &mut Window,
        cx: &mut App,
    ) -> Entity<TerminalView> {
        let (link, peer) = terminalview::Link::open();
        let view = cx.new(|cx| TerminalView::new(link, self.settings.borrow().clone(), window, cx));
        let (requests, source) = (self.requests.clone(), pane.clone());
        cx.subscribe(&view, move |_, event: &terminalview::Event, _| {
            Self::forward(&requests, &source, event);
        })
        .detach();
        let (frames, messages) = peer.into_channels();
        let reports = Self::report_into_shell(self.requests.clone(), pane.clone(), cx);
        let connection = Connection {
            pane: pane.clone(),
            ends: Ends { frames, messages },
            reports,
        };
        self.connect(owner, connection);
        self.views.borrow_mut().insert(pane.clone(), view.clone());
        view
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::Layout;

    use super::*;
    use crate::test_support::{heard_terminals, idle_terminals, terminal_layout};

    const DAEMON_WAIT: std::time::Duration = std::time::Duration::from_secs(10);

    fn panes_for(layout: &Layout) -> TerminalPanes {
        let ids = TerminalIds::default();
        ids.follow(layout);
        idle_terminals(ids)
    }

    #[gpui::test]
    fn a_terminal_pane_gets_one_view_and_keeps_it(cx: &mut TestAppContext) {
        cx.executor().allow_parking();
        cx.update(|cx| ui::init(cx).unwrap());
        let panes = panes_for(&terminal_layout(Some("c1")));
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new("t1").unwrap();
        let first = cx.update(|window, cx| panes.view(&pane, window, cx));
        let second = cx.update(|window, cx| panes.view(&pane, window, cx));
        assert_eq!(
            first.map(|view| view.entity_id()),
            second.map(|view| view.entity_id())
        );
        assert!(panes.is_terminal(&pane), "followed from the layout");
    }

    #[gpui::test]
    fn a_focused_terminal_pane_takes_the_keyboard(cx: &mut TestAppContext) {
        cx.executor().allow_parking();
        cx.update(|cx| ui::init(cx).unwrap());
        let panes = panes_for(&terminal_layout(Some("c1")));
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new("t1").unwrap();
        cx.update(|window, cx| panes.focus(&pane, window, cx));
        let terminal = cx
            .update(|window, cx| panes.view(&pane, window, cx))
            .unwrap();
        let focused = cx.update(|window, cx| terminal.focus_handle(cx).is_focused(window));
        assert!(focused, "the terminal holds the keyboard");
    }

    #[gpui::test]
    fn a_terminal_beside_a_chat_starts_a_session_of_its_own(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let ids = TerminalIds::default();
        ids.follow(&terminal_layout(Some("c1")));
        let (heard, requests) = std::sync::mpsc::channel();
        let panes = heard_terminals(ids, heard);
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new("t1").unwrap();
        cx.update(|window, cx| panes.view(&pane, window, cx));
        let first = requests.recv_timeout(DAEMON_WAIT).unwrap();
        let created = ptysession::SessionPaths::chat("c1").unwrap();
        assert_eq!(
            first,
            format!("POST {}", created.create_path()),
            "the chat's terminal asked for a new shell, not a session already running"
        );
    }

    #[gpui::test]
    fn other_panes_get_no_terminal(cx: &mut TestAppContext) {
        let panes = panes_for(&terminal_layout(None));
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let other = PaneId::new("p9").unwrap();
        assert!(
            cx.update(|window, cx| panes.view(&other, window, cx))
                .is_none()
        );
    }

    struct Shown(gpui::AnyView);

    impl gpui::Render for Shown {
        fn render(
            &mut self,
            _window: &mut Window,
            _cx: &mut gpui::Context<'_, Self>,
        ) -> impl gpui::IntoElement {
            self.0.clone()
        }
    }

    #[gpui::test]
    fn a_larger_font_applied_live_shrinks_the_grid(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let panes = panes_for(&terminal_layout(Some("c1")));
        let pane = PaneId::new("t1").unwrap();
        let (shown, cx) =
            cx.add_window_view(|window, cx| Shown(panes.view(&pane, window, cx).unwrap().into()));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear(cx));
        let terminal = cx.read_entity(&shown, |shown, _| {
            shown.0.clone().downcast::<TerminalView>().unwrap()
        });
        let before = cx.read_entity(&terminal, |view, _| view.grid_size());
        let larger = terminalview::Settings {
            font_size: 28.0,
            ..terminalview::Settings::default()
        };
        cx.update(|_, cx| panes.apply(larger, cx));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear(cx));
        let after = cx.read_entity(&terminal, |view, _| view.grid_size());
        assert!(
            after.0 < before.0,
            "{after:?} should have fewer columns than {before:?}"
        );
    }
}
