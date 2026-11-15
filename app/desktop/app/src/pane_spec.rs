use model::{Command, Layout, PaneInstance};
use protocol::{PaneState, PaneTypeId, Params};
use serde_json::json;

use crate::core_panes::{BROWSER, CONSOLE, EDITOR, REVIEW, TERMINAL};
use crate::error::Result;
use crate::pane_opener::open_commands;
use crate::serials::Serials;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct PaneSpec {
    pub(crate) type_id: PaneTypeId,
    pub(crate) params: Params,
    pub(crate) state: PaneState,
}

impl PaneSpec {
    pub(crate) fn open_commands(
        self,
        layout: &Layout,
        serials: &Serials,
    ) -> Result<Vec<Command>> {
        let mut commands = open_commands(layout, (self.type_id, self.params), serials)?;
        for instance in commands.iter_mut().filter_map(opened_instance) {
            instance.state = self.state.clone();
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

    pub(crate) fn of(instance: &PaneInstance) -> Self {
        Self {
            type_id: instance.type_id.clone(),
            params: instance.params.clone(),
            state: instance.state.clone(),
        }
    }

    pub(crate) fn editor(
        chat: &str,
        path: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with("threadId", chat)?
            .with("fileId", file_id(path))?;
        Self::new(EDITOR, params, PaneState::new(json!({ "path": path }))?)
    }

    pub(crate) fn untitled(
        chat: &str,
        id: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with("threadId", chat)?
            .with("fileId", format!("u{id}"))?;
        Self::new(EDITOR, params, PaneState::new(json!({ "untitled": true }))?)
    }

    pub(crate) fn host_file(
        chat: &str,
        path: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with("threadId", chat)?
            .with("fileId", file_id(path))?;
        let state = PaneState::new(json!({ "path": path, "absolute": true }))?;
        Self::new(EDITOR, params, state)
    }

    pub(crate) fn review(chat: &str) -> Result<Self> {
        let params = Params::default().with("threadId", chat)?;
        Self::new(REVIEW, params, PaneState::default())
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
            Some(chat) => params.with("threadId", chat)?,
            None => params,
        };
        let state = directory.map_or_else(PaneState::default, |dir| {
            PaneState::new(json!({ "directory": dir })).unwrap_or_default()
        });
        Self::new(TERMINAL, params, state)
    }

    pub(crate) fn browser(id: &str) -> Result<Self> {
        let params = Params::default().with("browserId", id)?;
        Self::new(BROWSER, params, PaneState::default())
    }
}

fn opened_instance(command: &mut Command) -> Option<&mut PaneInstance> {
    match command {
        Command::OpenPane { instance, .. } => Some(instance),
        _ => None,
    }
}

// SPEC: FNV-1a, fixed because the id is saved with the layout and must not change.
fn file_id(path: &str) -> String {
    let hash = path.bytes().fold(0xcbf2_9ce4_8422_2325_u64, |hash, byte| {
        (hash ^ u64::from(byte)).wrapping_mul(0x0100_0000_01b3)
    });
    format!("f{hash:016x}")
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (
        &'static str,
        fn() -> Result<PaneSpec>,
        &'static str,
        &'static str,
    );

    const CASES: [Case; 4] = [
        (
            "editor",
            || PaneSpec::editor("c1", "src/main.rs"),
            "path",
            "\"src/main.rs\"",
        ),
        (
            "untitled",
            || PaneSpec::untitled("c1", "p4"),
            "untitled",
            "true",
        ),
        (
            "host file",
            || PaneSpec::host_file("c1", "/home/a.rs"),
            "absolute",
            "true",
        ),
        (
            "terminal",
            || PaneSpec::terminal("t1", None, Some("/work")),
            "directory",
            "\"/work\"",
        ),
    ];

    #[test]
    fn a_spec_carries_what_it_needs_in_its_state() {
        for (name, spec, key, expected) in CASES {
            let state = spec().unwrap().state;
            assert_eq!(state.as_value()[key].to_string(), expected, "{name}");
        }
    }

    #[test]
    fn a_file_is_identified_by_its_path_without_a_slash_in_the_params() {
        let a = PaneSpec::editor("c1", "src/main.rs").unwrap();
        let again = PaneSpec::editor("c1", "src/main.rs").unwrap();
        let other = PaneSpec::editor("c1", "src/lib.rs").unwrap();
        assert_eq!(a.params, again.params, "same file, same pane");
        assert_ne!(a.params, other.params, "other file, other pane");
    }

    #[test]
    fn untitled_and_picked_files_are_told_apart_from_workspace_files() {
        let untitled = PaneSpec::untitled("c1", "p4").unwrap();
        let picked = PaneSpec::host_file("c1", "/home/a.rs").unwrap();
        assert_ne!(untitled.params, picked.params, "ids");
    }

    #[test]
    fn a_very_long_path_still_fits_the_params() {
        let path = "a/".repeat(2000);
        assert!(PaneSpec::editor("c1", &path).is_ok());
    }

    #[test]
    fn a_new_pane_starts_with_its_state_and_an_open_one_is_only_focused() {
        let serials = Serials::above(&Layout::default());
        let mut layout = Layout::default();
        let spec = PaneSpec::editor("c1", "a.rs").unwrap();
        for command in spec.clone().open_commands(&layout, &serials).unwrap() {
            layout.apply(command).unwrap();
        }
        let opened = layout.find_pane(&spec.type_id, &spec.params).unwrap();
        assert_eq!(opened.state, spec.state, "state");
        let again = spec.open_commands(&layout, &serials).unwrap();
        assert!(
            again.iter().all(|c| !matches!(c, Command::OpenPane { .. })),
            "no second pane"
        );
    }

    #[test]
    fn file_ids_are_stable() {
        assert_eq!(file_id("a"), "faf63dc4c8601ec8c");
    }

    #[test]
    fn one_console_and_a_review_per_chat() {
        assert_eq!(PaneSpec::console().unwrap().params, Params::default());
        let review = PaneSpec::review("c1").unwrap();
        assert_eq!(review.params.get("threadId"), Some("c1"), "chat");
    }

    #[test]
    fn a_terminal_without_a_directory_starts_with_the_default_state() {
        assert_eq!(
            PaneSpec::terminal("t2", None, None).unwrap().state,
            PaneState::default()
        );
    }
}
