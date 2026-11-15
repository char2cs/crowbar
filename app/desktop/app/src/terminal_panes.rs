use std::cell::RefCell;
use std::collections::BTreeMap;

use gpui::{AnyView, App, AppContext as _, Entity, Window};
use protocol::PaneId;
use terminal::DaemonFrame;
use terminal_session::{Ends, SessionBase, TerminalSessions};
use terminal_view::{TerminalLink, TerminalSettings, TerminalView};

use crate::terminal_ids::TerminalIds;

const NO_CHAT: &str = "Open a chat first: a terminal runs in a chat's workspace.\r\n";

pub(crate) struct TerminalPanes {
    ids: TerminalIds,
    sessions: TerminalSessions,
    views: RefCell<BTreeMap<PaneId, Entity<TerminalView>>>,
    settings: RefCell<TerminalSettings>,
}

impl TerminalPanes {
    pub(crate) fn new(
        ids: TerminalIds,
        sessions: TerminalSessions,
    ) -> Self {
        Self {
            ids,
            sessions,
            views: RefCell::default(),
            settings: RefCell::default(),
        }
    }

    pub(crate) fn apply(
        &self,
        settings: TerminalSettings,
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

    pub(crate) fn view(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<AnyView> {
        self.views
            .borrow_mut()
            .retain(|open, _| self.ids.contains(open));
        if !self.ids.contains(pane) {
            return None;
        }
        let chat = self.ids.chat_of(pane);
        let existing = self.views.borrow().get(pane).cloned();
        let view = existing.unwrap_or_else(|| self.create((pane, chat.as_deref()), window, cx));
        Some(view.into())
    }

    fn create(
        &self,
        (pane, chat): (&PaneId, Option<&str>),
        window: &mut Window,
        cx: &mut App,
    ) -> Entity<TerminalView> {
        let (link, peer) = TerminalLink::open();
        let view = cx.new(|cx| TerminalView::new(link, self.settings.borrow().clone(), window, cx));
        let (frames, messages) = peer.into_channels();
        self.connect(pane, chat, Ends { frames, messages });
        self.views.borrow_mut().insert(pane.clone(), view.clone());
        view
    }

    fn connect(
        &self,
        pane: &PaneId,
        chat: Option<&str>,
        ends: Ends,
    ) {
        match chat.map(SessionBase::chat) {
            Some(Ok(base)) => self.sessions.start(base, pane.clone(), ends),
            _ => notify(&ends, NO_CHAT),
        }
    }
}

fn notify(
    ends: &Ends,
    text: &str,
) {
    let frame = DaemonFrame::Output(text.as_bytes().to_vec());
    if ends.frames.try_send(frame).is_err() {
        tracing::debug!("the terminal closed before its notice");
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use model::{Command, Layout, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::core_panes::TERMINAL;
    use crate::test_support::idle_terminals;

    fn layout_with_terminal(chat: Option<&str>) -> Layout {
        let mut layout = Layout::default();
        let tab = TabId(1);
        let add = Command::AddTab {
            id: tab,
            title: String::new(),
        };
        layout.apply(add).unwrap();
        let params = chat.map_or_else(Params::default, |chat| {
            Params::default().with("threadId", chat).unwrap()
        });
        let instance = PaneInstance {
            pane_id: PaneId::new("t1").unwrap(),
            type_id: PaneTypeId::new(TERMINAL).unwrap(),
            params,
            state: PaneState::default(),
        };
        let placement = None;
        let open = Command::OpenPane {
            tab,
            instance,
            placement,
        };
        layout.apply(open).unwrap();
        layout
    }

    fn panes_for(layout: &Layout) -> TerminalPanes {
        let ids = TerminalIds::default();
        ids.follow(layout);
        idle_terminals(ids)
    }

    #[gpui::test]
    fn a_terminal_pane_gets_one_view_and_keeps_it(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let panes = panes_for(&layout_with_terminal(Some("c1")));
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
    fn other_panes_get_no_terminal(cx: &mut TestAppContext) {
        let panes = panes_for(&layout_with_terminal(None));
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let other = PaneId::new("p9").unwrap();
        assert!(
            cx.update(|window, cx| panes.view(&other, window, cx))
                .is_none()
        );
    }

    struct Shown(AnyView);

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
        let panes = panes_for(&layout_with_terminal(Some("c1")));
        let pane = PaneId::new("t1").unwrap();
        let (shown, cx) =
            cx.add_window_view(|window, cx| Shown(panes.view(&pane, window, cx).unwrap()));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear());
        let terminal = cx.read_entity(&shown, |shown, _| {
            shown.0.clone().downcast::<TerminalView>().unwrap()
        });
        let before = cx.read_entity(&terminal, |view, _| view.grid_size());
        let larger = TerminalSettings {
            font_size: 28.0,
            ..TerminalSettings::default()
        };
        cx.update(|_, cx| panes.apply(larger, cx));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear());
        let after = cx.read_entity(&terminal, |view, _| view.grid_size());
        assert!(
            after.0 < before.0,
            "{after:?} should have fewer columns than {before:?}"
        );
    }
}
