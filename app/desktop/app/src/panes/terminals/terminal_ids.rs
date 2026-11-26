use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use layout::{Layout, PaneInstance, Tab};
use protocol::{PaneId, PaneState, PaneTitle, PaneTypeId};

use crate::panes::kinds::core_panes::TERMINAL;
use crate::panes::terminals::terminal_owner::TerminalOwner;
use crate::panes::tracking::titles::Titles;

const DEFAULT_LABEL: &str = "Terminal";
pub(crate) const NAME_KEY: &str = "title";
const BUFFER_NAME_KEY: &str = "name";
pub(crate) const SESSION_KEY: &str = "sessionId";

type Chats = BTreeMap<PaneId, Option<TerminalOwner>>;
type Labels = BTreeMap<PaneId, String>;

#[derive(Debug, Clone, Default)]
pub(crate) struct TerminalIds {
    panes: Rc<RefCell<Chats>>,
    labels: Rc<RefCell<Labels>>,
    sessions: Rc<RefCell<Labels>>,
}

impl TerminalIds {
    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.borrow().contains_key(pane)
    }

    pub(crate) fn owner_of(
        &self,
        pane: &PaneId,
    ) -> Option<TerminalOwner> {
        self.panes.borrow().get(pane).cloned().flatten()
    }

    pub(crate) fn saved_session(
        &self,
        pane: &PaneId,
    ) -> Option<String> {
        self.sessions.borrow().get(pane).cloned()
    }

    pub(crate) fn follow(
        &self,
        layout: &Layout,
    ) {
        let terminals = terminals_in(layout);
        let chats = terminals
            .iter()
            .map(|(pane, instance)| (pane.clone(), TerminalOwner::of(&instance.params)));
        let labels = terminals
            .iter()
            .map(|(pane, instance)| (pane.clone(), label_of(instance)));
        let sessions = terminals.iter().filter_map(|(pane, instance)| {
            let saved = instance.state.as_value().get(SESSION_KEY)?.as_str()?;
            Some((pane.clone(), saved.to_owned()))
        });
        *self.panes.borrow_mut() = chats.collect();
        *self.labels.borrow_mut() = labels.collect();
        *self.sessions.borrow_mut() = sessions.collect();
    }

    pub(crate) fn name_into(
        &self,
        titles: &Titles,
    ) {
        let labels = self.labels.borrow();
        let untitled = labels.iter().filter(|(pane, _)| titles.get(pane).is_none());
        for (pane, label) in untitled {
            name(titles, pane, label);
        }
    }
}

pub(crate) fn opened_state(
    type_id: &PaneTypeId,
    state: &PaneState,
    layout: &Layout,
) -> PaneState {
    if type_id.as_str() != TERMINAL {
        return state.clone();
    }
    let mut fields = state.as_value().as_object().cloned().unwrap_or_default();
    if fields.contains_key(BUFFER_NAME_KEY) {
        return state.clone();
    }
    let number = terminals_in(layout).len() + 1;
    let name = format!("{DEFAULT_LABEL} {number}");
    fields.insert(BUFFER_NAME_KEY.to_owned(), serde_json::Value::from(name));
    PaneState::new(serde_json::Value::Object(fields)).unwrap_or_else(|_| state.clone())
}

fn name(
    titles: &Titles,
    pane: &PaneId,
    label: &str,
) {
    if let Ok(title) = PaneTitle::new(label) {
        titles.set(pane.clone(), title);
    }
}

fn terminals_in(layout: &Layout) -> Vec<(PaneId, &PaneInstance)> {
    let terminal = |pane: PaneId| {
        let instance = layout.pane(&pane)?;
        (instance.type_id.as_str() == TERMINAL).then_some((pane, instance))
    };
    layout
        .tabs()
        .iter()
        .flat_map(Tab::pane_ids)
        .filter_map(terminal)
        .collect()
}

fn label_of(instance: &PaneInstance) -> String {
    let state = instance.state.as_value();
    let text = |key: &str| state.get(key).and_then(serde_json::Value::as_str);
    let named = text(NAME_KEY).filter(|name| !name.is_empty());
    let folder = text("directory").and_then(|path| path.rsplit('/').find(|part| !part.is_empty()));
    let opened_as = text(BUFFER_NAME_KEY);
    named
        .or(folder)
        .or(opened_as)
        .unwrap_or(DEFAULT_LABEL)
        .to_owned()
}

#[cfg(test)]
mod tests {
    use layout::{Command, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn ensure_tab(
        layout: &mut Layout,
        tab: TabId,
    ) {
        if !layout.tabs().is_empty() {
            return;
        }
        let add = Command::AddTab {
            id: tab,
            title: String::new(),
            space: layout::SpaceId::default(),
        };
        layout.apply(add).unwrap();
    }

    fn open(
        layout: &mut Layout,
        (pane, type_id): (&str, &str),
        params: Params,
    ) {
        let instance = PaneInstance {
            pane_id: PaneId::new(pane).unwrap(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params,
            state: PaneState::default(),
        };
        let tab = TabId(1);
        ensure_tab(layout, tab);
        let placement = None;
        let command = Command::OpenPane {
            tab,
            instance,
            placement,
        };
        layout.apply(command).unwrap();
    }

    #[test]
    fn terminal_panes_are_followed_with_their_chat() {
        let mut layout = Layout::default();
        let chat = Params::default().with("threadId", "c1").unwrap();
        open(&mut layout, ("t1", TERMINAL), chat);
        open(&mut layout, ("t2", TERMINAL), Params::default());
        open(&mut layout, ("p3", "core.chat"), Params::default());
        let ids = TerminalIds::default();
        ids.follow(&layout);
        let (t1, t2) = (PaneId::new("t1").unwrap(), PaneId::new("t2").unwrap());
        assert_eq!(
            ids.owner_of(&t1),
            Some(TerminalOwner::Chat("c1".to_owned())),
            "with chat"
        );
        assert_eq!(ids.owner_of(&t2), None, "without chat");
        assert!(ids.contains(&t2), "still a terminal");
        assert!(
            !ids.contains(&PaneId::new("p3").unwrap()),
            "a chat is no terminal"
        );
    }

    #[test]
    fn a_closed_terminal_is_forgotten() {
        let mut layout = Layout::default();
        open(&mut layout, ("t1", TERMINAL), Params::default());
        let ids = TerminalIds::default();
        ids.follow(&layout);
        layout
            .apply(Command::ClosePane {
                pane: PaneId::new("t1").unwrap(),
            })
            .unwrap();
        ids.follow(&layout);
        assert!(!ids.contains(&PaneId::new("t1").unwrap()));
    }

    #[test]
    fn a_terminal_remembers_the_session_it_was_bound_to() {
        let mut layout = Layout::default();
        open_with_state(&mut layout, serde_json::json!({ "sessionId": "s7" }));
        let ids = TerminalIds::default();
        ids.follow(&layout);
        let pane = PaneId::new("t1").unwrap();
        assert_eq!(ids.saved_session(&pane).as_deref(), Some("s7"));
    }

    type LabelCase = (&'static str, Option<&'static str>, &'static str);

    const CASES: [LabelCase; 4] = [
        ("a folder", Some("/work/src"), "src"),
        ("a trailing slash", Some("docs/"), "docs"),
        ("the root", Some("/"), "Terminal"),
        ("no folder", None, "Terminal"),
    ];

    fn open_in(
        layout: &mut Layout,
        directory: Option<&str>,
    ) {
        let state = directory.map_or_else(PaneState::default, |dir| {
            PaneState::new(serde_json::json!({ "directory": dir })).unwrap()
        });
        open_pane_with(layout, state);
    }

    fn open_with_state(
        layout: &mut Layout,
        state: serde_json::Value,
    ) {
        open_pane_with(layout, PaneState::new(state).unwrap());
    }

    fn open_pane_with(
        layout: &mut Layout,
        state: PaneState,
    ) {
        let instance = PaneInstance {
            pane_id: PaneId::new("t1").unwrap(),
            type_id: PaneTypeId::new(TERMINAL).unwrap(),
            params: Params::default(),
            state,
        };
        let tab = TabId(1);
        ensure_tab(layout, tab);
        let placement = None;
        layout
            .apply(Command::OpenPane {
                tab,
                instance,
                placement,
            })
            .unwrap();
    }

    #[test]
    fn terminals_are_titled_by_their_folder() {
        for (name, directory, expected) in CASES {
            let mut layout = Layout::default();
            open_in(&mut layout, directory);
            let (ids, titles) = (TerminalIds::default(), Titles::default());
            ids.follow(&layout);
            ids.name_into(&titles);
            let pane = PaneId::new("t1").unwrap();
            assert_eq!(
                titles.get(&pane).map(|t| t.to_string()),
                Some(expected.into()),
                "{name}"
            );
        }
    }

    #[test]
    fn a_name_the_user_gave_wins_over_the_folder() {
        let mut layout = Layout::default();
        open_with_state(
            &mut layout,
            serde_json::json!({"directory": "/w/src", "title": "logs"}),
        );
        let (ids, titles) = (TerminalIds::default(), Titles::default());
        ids.follow(&layout);
        ids.name_into(&titles);
        let pane = PaneId::new("t1").unwrap();
        assert_eq!(
            titles.get(&pane).map(|t| t.to_string()),
            Some("logs".into())
        );
    }

    #[test]
    fn a_title_the_terminal_already_has_is_kept() {
        let mut layout = Layout::default();
        open_in(&mut layout, Some("/work/src"));
        let (ids, titles) = (TerminalIds::default(), Titles::default());
        let pane = PaneId::new("t1").unwrap();
        titles.set(pane.clone(), PaneTitle::new("vim").unwrap());
        ids.follow(&layout);
        ids.name_into(&titles);
        assert_eq!(titles.get(&pane).map(|t| t.to_string()), Some("vim".into()));
    }
}
