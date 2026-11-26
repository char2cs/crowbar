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
    use layout::Command;

    use super::*;
    use crate::test_support::{chat_pane, one_tab, pane};

    fn terminal(state: &str) -> Layout {
        let state = PaneState::new(serde_json::from_str(state).unwrap()).unwrap();
        one_tab([PaneInstance {
            state,
            ..pane("t1", TERMINAL)
        }])
    }

    #[test]
    fn terminal_panes_are_followed_with_their_chat_until_they_close() {
        let mut layout = one_tab([
            chat_pane(("t1", TERMINAL), "c1"),
            pane("t2", TERMINAL),
            pane("p3", "core.chat"),
        ]);
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
        layout
            .apply(Command::ClosePane { pane: t1.clone() })
            .unwrap();
        ids.follow(&layout);
        assert!(!ids.contains(&t1), "a closed terminal is forgotten");
    }

    #[test]
    fn a_terminal_remembers_the_session_it_was_bound_to() {
        let ids = TerminalIds::default();
        ids.follow(&terminal(r#"{"sessionId": "s7"}"#));
        let pane = PaneId::new("t1").unwrap();
        assert_eq!(ids.saved_session(&pane).as_deref(), Some("s7"));
    }

    type LabelCase = (
        &'static str,
        &'static str,
        Option<&'static str>,
        &'static str,
    );

    const CASES: [LabelCase; 6] = [
        ("a folder", r#"{"directory": "/work/src"}"#, None, "src"),
        (
            "a trailing slash",
            r#"{"directory": "docs/"}"#,
            None,
            "docs",
        ),
        ("the root", r#"{"directory": "/"}"#, None, "Terminal"),
        ("no folder", "null", None, "Terminal"),
        (
            "a name the user gave wins over the folder",
            r#"{"directory": "/w/src", "title": "logs"}"#,
            None,
            "logs",
        ),
        (
            "a title the terminal already has is kept",
            r#"{"directory": "/work/src"}"#,
            Some("vim"),
            "vim",
        ),
    ];

    fn holding(
        pane: &PaneId,
        held: Option<&str>,
    ) -> Titles {
        let titles = Titles::default();
        let Some(held) = held else {
            return titles;
        };
        titles.set(pane.clone(), PaneTitle::new(held).unwrap());
        titles
    }

    #[test]
    fn terminals_are_titled_by_their_folder() {
        for (name, state, held, expected) in CASES {
            let pane = PaneId::new("t1").unwrap();
            let (ids, titles) = (TerminalIds::default(), holding(&pane, held));
            ids.follow(&terminal(state));
            ids.name_into(&titles);
            assert_eq!(
                titles.get(&pane).map(|t| t.to_string()),
                Some(expected.into()),
                "{name}"
            );
        }
    }
}
