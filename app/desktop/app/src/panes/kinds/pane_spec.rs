use layout::{Command, Layout, PaneInstance};
use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde_json::json;

use crate::error::Result;
use crate::panes::kinds::core_panes::{BROWSER, CONSOLE, NEW_TAB, REVIEW, TERMINAL};
use crate::panes::kinds::pane_opener::open_commands;
use crate::panes::kinds::serials::Serials;
use crate::panes::terminals::terminal_ids::opened_state;

mod file_specs;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct PaneSpec {
    pub(crate) type_id: PaneTypeId,
    pub(crate) params: Params,
    pub(crate) state: PaneState,
}

impl PaneSpec {
    pub(crate) fn open_commands(
        self,
        (layout, space): (&Layout, &str),
        serials: &Serials,
    ) -> Result<Vec<Command>> {
        let state = opened_state(&self.type_id, &self.state, layout);
        let mut commands = open_commands(layout, space, (self.type_id, self.params), serials)?;
        for instance in commands.iter_mut().filter_map(opened_instance) {
            instance.state = state.clone();
        }
        Ok(commands)
    }

    fn new(
        type_id: &str,
        params: Params,
        state: PaneState,
    ) -> Result<Self> {
        Ok(Self {
            type_id: PaneTypeId::new(type_id)?,
            params,
            state,
        })
    }

    pub(crate) fn instance(
        self,
        pane_id: PaneId,
    ) -> PaneInstance {
        PaneInstance {
            pane_id,
            type_id: self.type_id,
            params: self.params,
            state: self.state,
        }
    }

    pub(crate) fn of(instance: &PaneInstance) -> Self {
        Self {
            type_id: instance.type_id.clone(),
            params: instance.params.clone(),
            state: instance.state.clone(),
        }
    }

    pub(crate) fn review(
        chat: &str,
        reveal: Option<&str>,
    ) -> Result<Self> {
        let params = Params::default().with(THREAD_ID, chat)?;
        let state = reveal.map(|path| PaneState::new(json!({ "reveal": path })));
        Self::new(REVIEW, params, state.transpose()?.unwrap_or_default())
    }

    pub(crate) fn console() -> Result<Self> {
        Self::new(CONSOLE, Params::default(), PaneState::default())
    }

    pub(crate) fn terminal(
        id: &str,
        chat: Option<&str>,
        directory: Option<&str>,
    ) -> Result<Self> {
        let params = Params::default().with("terminalId", id)?;
        let params = match chat {
            Some(chat) => params.with(THREAD_ID, chat)?,
            None => params,
        };
        let state = directory.map_or_else(PaneState::default, |dir| {
            PaneState::new(json!({ "directory": dir })).unwrap_or_default()
        });
        Self::new(TERMINAL, params, state)
    }

    pub(crate) fn home_terminal(
        id: &str,
        project: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with("terminalId", id)?
            .with("projectId", project)?;
        Self::new(TERMINAL, params, PaneState::default())
    }

    pub(crate) fn new_tab(
        id: &str,
        chat: Option<&str>,
    ) -> Result<Self> {
        let params = Params::default().with("newTabId", id)?;
        let params = match chat {
            Some(chat) => params.with(THREAD_ID, chat)?,
            None => params,
        };
        Self::new(NEW_TAB, params, PaneState::default())
    }

    pub(crate) fn browser(id: &str) -> Result<Self> {
        let params = Params::default().with("browserId", id)?;
        Self::new(BROWSER, params, PaneState::default())
    }

    pub(crate) fn agent_browser(
        id: &str,
        chat: &str,
        url: Option<&str>,
    ) -> Result<Self> {
        let params = Params::default()
            .with("browserId", id)?
            .with(THREAD_ID, chat)?;
        let state = match url {
            Some(url) => PaneState::new(json!({ "url": url }))?,
            None => PaneState::default(),
        };
        Self::new(BROWSER, params, state)
    }
}

const THREAD_ID: &str = "threadId";

fn opened_instance(command: &mut Command) -> Option<&mut PaneInstance> {
    match command {
        Command::OpenPane { instance, .. } | Command::OpenPaneInGroup { instance, .. } => {
            Some(instance)
        },
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_browser_an_agent_opens_names_its_chat_and_its_first_page() {
        let spec = PaneSpec::agent_browser("b1", "c1", Some("http://a.test/")).unwrap();
        assert_eq!(spec.params.get("threadId"), Some("c1"));
        assert_eq!(spec.state.as_value()["url"], "http://a.test/");
        let blank = PaneSpec::agent_browser("b2", "c1", None).unwrap();
        assert_eq!(blank.state, PaneState::default());
    }

    #[test]
    fn a_new_pane_starts_with_its_state_and_an_open_one_is_only_focused() {
        let serials = Serials::above(&Layout::default());
        let mut layout = Layout::default();
        let spec = PaneSpec::editor("c1", "a.rs").unwrap();
        for command in spec.clone().open_commands((&layout, ""), &serials).unwrap() {
            layout.apply(command).unwrap();
        }
        let opened = layout.find_pane(&spec.type_id, &spec.params).unwrap();
        assert_eq!(opened.state, spec.state, "state");
        let again = spec.open_commands((&layout, ""), &serials).unwrap();
        assert!(
            again.iter().all(|c| !matches!(c, Command::OpenPane { .. })),
            "no second pane"
        );
    }

    fn open_into(
        layout: &mut Layout,
        spec: &PaneSpec,
        serials: &Serials,
    ) {
        for command in spec.clone().open_commands((layout, ""), serials).unwrap() {
            layout.apply(command).unwrap();
        }
    }

    #[test]
    fn a_pane_stacked_into_a_group_starts_with_its_state_too() {
        let serials = Serials::above(&Layout::default());
        let mut layout = Layout::default();
        let chat = PaneSpec {
            type_id: PaneTypeId::new(crate::panes::kinds::core_panes::CHAT).unwrap(),
            params: crate::panes::kinds::core_panes::chat_params("c1").unwrap(),
            state: PaneState::default(),
        };
        open_into(&mut layout, &chat, &serials);
        for path in ["a.rs", "b.rs"] {
            let spec = PaneSpec::editor("c1", path).unwrap();
            open_into(&mut layout, &spec, &serials);
            let opened = layout.find_pane(&spec.type_id, &spec.params).unwrap();
            assert_eq!(opened.state, spec.state, "{path}");
        }
    }

    type Case = (
        &'static str,
        fn() -> Result<PaneSpec>,
        (&'static str, Option<&'static str>),
    );

    const CASES: [Case; 6] = [
        ("console", PaneSpec::console, ("threadId", None)),
        (
            "review",
            || PaneSpec::review("c1", None),
            ("threadId", Some("c1")),
        ),
        (
            "terminal",
            || PaneSpec::terminal("t1", None, None),
            ("terminalId", Some("t1")),
        ),
        (
            "home terminal",
            || PaneSpec::home_terminal("t1", "p1"),
            ("projectId", Some("p1")),
        ),
        (
            "new tab",
            || PaneSpec::new_tab("n1", None),
            ("newTabId", Some("n1")),
        ),
        (
            "browser",
            || PaneSpec::browser("b1"),
            ("browserId", Some("b1")),
        ),
    ];

    #[test]
    fn each_kind_of_pane_identifies_itself_in_its_params() {
        for (name, spec, (key, expected)) in CASES {
            assert_eq!(spec().unwrap().params.get(key), expected, "{name}");
        }
    }

    #[test]
    fn terminal_specs_carry_their_directory_or_the_default_state() {
        let placed = PaneSpec::terminal("t1", None, Some("/work")).unwrap();
        assert_eq!(placed.state.as_value()["directory"], "/work", "directory");
        let bare = PaneSpec::terminal("t2", None, None).unwrap();
        assert_eq!(bare.state, PaneState::default(), "default");
    }
}
