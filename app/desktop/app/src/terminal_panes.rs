use std::cell::RefCell;
use std::collections::{BTreeMap, BTreeSet};

use gpui::{AnyView, App, AppContext as _, Entity, Window};
use protocol::PaneId;
use ptysession::{Ends, TerminalSessions};
use terminal::DaemonFrame;
use terminalview::TerminalView;

use crate::chat_terminals::ChatTerminals;
use crate::plan::Plan;
use crate::terminal_ids::TerminalIds;
use crate::terminal_owner::TerminalOwner;

const STARTING: &str = "Starting the chat's terminal...\r\n";
const NO_CHAT: &str = "Open a chat first: a terminal runs in a chat's workspace.\r\n";

pub(crate) struct TerminalPanes {
    ids: TerminalIds,
    sessions: TerminalSessions,
    chats: ChatTerminals,
    views: RefCell<BTreeMap<PaneId, Entity<TerminalView>>>,
    waiting: RefCell<BTreeSet<PaneId>>,
    settings: RefCell<terminalview::Settings>,
}

impl TerminalPanes {
    pub(crate) fn new(
        ids: TerminalIds,
        (sessions, chats): (TerminalSessions, ChatTerminals),
    ) -> Self {
        Self {
            ids,
            sessions,
            chats,
            views: RefCell::default(),
            waiting: RefCell::default(),
            settings: RefCell::default(),
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
        let owner = self.ids.owner_of(pane);
        let existing = self.views.borrow().get(pane).cloned();
        let view = existing.unwrap_or_else(|| self.create((pane, owner.as_ref()), window, cx));
        Some(view.into())
    }

    fn create(
        &self,
        (pane, owner): (&PaneId, Option<&TerminalOwner>),
        window: &mut Window,
        cx: &mut App,
    ) -> Entity<TerminalView> {
        let (link, peer) = terminalview::Link::open();
        let view = cx.new(|cx| TerminalView::new(link, self.settings.borrow().clone(), window, cx));
        let (frames, messages) = peer.into_channels();
        self.connect((pane, owner), Ends { frames, messages });
        self.views.borrow_mut().insert(pane.clone(), view.clone());
        view
    }

    fn connect(
        &self,
        (pane, owner): (&PaneId, Option<&TerminalOwner>),
        ends: Ends,
    ) {
        let base = owner.map(TerminalOwner::base);
        let (Some(owner), Some(Ok(base))) = (owner, base) else {
            notify(&ends, NO_CHAT);
            return;
        };
        let pane = pane.clone();
        match self.plan_for(owner) {
            Plan::Own => self.sessions.start(base, pane, ends),
            Plan::Watch(session) => self.sessions.watch((base, session), pane, ends),
            Plan::Resume => self.resume(&pane, owner, &ends),
        }
    }

    fn plan_for(
        &self,
        owner: &TerminalOwner,
    ) -> Plan {
        match owner {
            TerminalOwner::Chat(chat) => Plan::of(self.chats.state(chat).as_ref()),
            TerminalOwner::Home(_) => Plan::Own,
        }
    }

    fn resume(
        &self,
        pane: &PaneId,
        owner: &TerminalOwner,
        ends: &Ends,
    ) {
        notify(ends, STARTING);
        self.waiting.borrow_mut().insert(pane.clone());
        if let TerminalOwner::Chat(chat) = owner {
            self.chats.resume(chat);
        }
    }

    pub(crate) fn attached(&self) -> bool {
        let ready: Vec<PaneId> = self
            .waiting
            .borrow()
            .iter()
            .filter(|pane| self.is_ready(pane))
            .cloned()
            .collect();
        for pane in &ready {
            self.waiting.borrow_mut().remove(pane);
            self.views.borrow_mut().remove(pane);
        }
        !ready.is_empty()
    }

    fn is_ready(
        &self,
        pane: &PaneId,
    ) -> bool {
        let owner = self.ids.owner_of(pane);
        owner.is_none_or(|owner| !matches!(self.plan_for(&owner), Plan::Resume))
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
    use std::rc::Rc;

    use gpui::TestAppContext;
    use layout::{Command, Layout, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};
    use store::{ChatPhase, ChatSurface, ChatTerminal};

    use super::*;
    use crate::core_panes::TERMINAL;
    use crate::test_support::{idle_terminals, idle_terminals_for};

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
        let larger = terminalview::Settings {
            font_size: 28.0,
            ..terminalview::Settings::default()
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

    fn dormant_then_live() -> (ChatTerminals, Rc<RefCell<ChatTerminal>>) {
        let state = Rc::new(RefCell::new(ChatTerminal {
            surface: ChatSurface::Terminal,
            live_runner_id: None,
            terminal_session_id: None,
            phase: ChatPhase::Dormant,
        }));
        let seen = Rc::clone(&state);
        let chats = ChatTerminals::new(
            Rc::new(move |_| Some(seen.borrow().clone())),
            Rc::new(|_| {}),
        );
        (chats, state)
    }

    #[gpui::test]
    fn a_dormant_chat_terminal_is_rebuilt_once_its_runner_appears(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (chats, state) = dormant_then_live();
        let ids = TerminalIds::default();
        ids.follow(&layout_with_terminal(Some("c1")));
        let panes = idle_terminals_for(ids, chats);
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new("t1").unwrap();
        let first = cx.update(|window, cx| panes.view(&pane, window, cx));
        assert!(!panes.attached(), "still dormant");
        let live = ChatTerminal {
            live_runner_id: Some("r".into()),
            terminal_session_id: Some("s1".into()),
            phase: ChatPhase::Live,
            ..state.borrow().clone()
        };
        *state.borrow_mut() = live;
        assert!(panes.attached(), "the runner appeared");
        let second = cx.update(|window, cx| panes.view(&pane, window, cx));
        assert_ne!(
            first.map(|view| view.entity_id()),
            second.map(|view| view.entity_id()),
            "a view bound to the new session"
        );
    }
}
