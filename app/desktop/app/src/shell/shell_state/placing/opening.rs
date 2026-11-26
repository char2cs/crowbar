use layout::{Placement, TabId};
use protocol::{PaneTypeId, Params};

use super::preview::keeping;
use crate::panes::kinds::core_panes::{THREAD_ID, chat_params, chat_type};
use crate::panes::kinds::pane_opener::{open_at, open_view_commands};
use crate::panes::kinds::pane_spec::PaneSpec;
use crate::shell::shell_state::ShellState;

impl ShellState {
    pub(crate) fn open_view(
        &mut self,
        type_id: PaneTypeId,
        params: Params,
    ) {
        self.open_with((type_id, params), Self::apply);
    }

    // SPEC: app/panes/src/features/agent/composer/composer-field.tsx: a chat mounts focused
    pub(crate) fn open_from_sidebar(
        &mut self,
        view: (PaneTypeId, Params),
    ) {
        let apply: fn(&mut Self, Vec<layout::Command>) =
            match self.history.layout().find_pane(&view.0, &view.1) {
                Some(_) => Self::apply_from_strip,
                None => Self::apply,
            };
        self.open_with(view, apply);
    }

    fn space_of(
        &self,
        params: &Params,
    ) -> String {
        let chat = params.get(THREAD_ID);
        let own = chat.and_then(|chat| self.space.of_chat(chat));
        own.unwrap_or_else(|| self.space.active())
    }

    fn open_with(
        &mut self,
        view: (PaneTypeId, Params),
        apply: fn(&mut Self, Vec<layout::Command>),
    ) {
        let space = self.space_of(&view.1);
        match open_view_commands(self.history.layout(), &space, view, &self.serials) {
            Ok(commands) => apply(self, commands),
            Err(error) => tracing::warn!(%error, "could not open the view"),
        }
    }

    pub(crate) fn open_chat_at(
        &mut self,
        chat: &str,
        (tab, placement): (TabId, Placement),
    ) {
        let opened = chat_type()
            .and_then(|type_id| Ok((type_id, chat_params(chat)?)))
            .and_then(|pane| open_at(pane, (tab, placement), &self.serials));
        match opened {
            Ok(command) => self.apply(vec![command]),
            Err(error) => tracing::warn!(%error, "could not open the dropped chat"),
        }
    }

    pub(crate) fn open_spec(
        &mut self,
        spec: PaneSpec,
    ) {
        match self.spec_commands(spec) {
            Ok(commands) => self.apply(keeping(self.history.layout(), commands)),
            Err(error) => tracing::warn!(%error, "could not open the pane"),
        }
    }

    pub(crate) fn reopen_closed(&mut self) -> bool {
        let Some(spec) = self.closed.take_last() else {
            return false;
        };
        self.open_spec(spec);
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{open_chat, page_takes_keys, resync, shell_rig, synced_chat};
    use layout::Tab;

    #[test]
    fn a_terminal_opened_from_a_folder_is_titled_by_it() {
        let cases = [("/work/src", "src"), ("/work/api", "api")];
        for (folder, expected) in cases {
            let mut rig = shell_rig();
            let spec = PaneSpec::terminal("t1", None, Some(folder)).unwrap();
            rig.shell.open_spec(spec);
            let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
            let title = rig.shell.titles().get(&pane).map(|t| t.to_string());
            assert_eq!(title.as_deref(), Some(expected), "{folder}");
        }
    }

    #[test]
    fn a_terminal_opened_without_a_folder_is_numbered_after_the_open_terminals() {
        let mut rig = shell_rig();
        for id in ["t1", "t2"] {
            rig.shell
                .open_spec(PaneSpec::terminal(id, None, None).unwrap());
        }
        let titles: Vec<_> = rig.shell.layout().tabs()[0]
            .pane_ids()
            .iter()
            .map(|pane| rig.shell.titles().get(pane).map(|t| t.to_string()))
            .collect();
        let expected = ["Terminal 1", "Terminal 2"].map(|name| Some(name.to_owned()));
        assert_eq!(titles, expected);
    }

    #[test]
    fn an_opened_chat_gets_its_own_view_and_an_open_one_is_revealed() {
        let mut rig = shell_rig();
        let open = |rig: &mut crate::test_support::ShellRig, chat: &str| {
            rig.shell
                .open_view(chat_type().unwrap(), chat_params(chat).unwrap());
        };
        open(&mut rig, "t1");
        open(&mut rig, "t2");
        assert_eq!(rig.shell.layout().tabs().len(), 2, "a view per chat");
        assert_eq!(rig.shell.layout().pane_count(), 2);
        open(&mut rig, "t1");
        assert_eq!(
            rig.shell.layout().tabs().len(),
            2,
            "t1 revealed, not copied"
        );
        let active = rig.shell.showing().map(Tab::id);
        assert_eq!(active, Some(rig.shell.layout().tabs()[0].id()));
    }

    #[test]
    fn a_chat_dropped_on_a_pane_opens_beside_it_and_an_open_one_moves() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        open_chat(&mut rig.shell, "t2");
        let first = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        let tab = rig.shell.layout().tabs()[0].id();
        let placement = Placement {
            target: first,
            edge: layout::Edge::Left,
        };
        rig.shell.open_chat_at("t3", (tab, placement.clone()));
        assert_eq!(rig.shell.layout().pane_count(), 3, "t3 opened");
        rig.shell.open_chat_at("t2", (tab, placement));
        assert_eq!(rig.shell.layout().pane_count(), 3, "t2 moved, not copied");
    }

    #[test]
    fn a_closed_file_tab_comes_back_once_and_a_closed_chat_never_does() {
        let mut rig = shell_rig();
        assert!(!rig.shell.reopen_closed(), "nothing closed yet");
        open_chat(&mut rig.shell, "t1");
        let chat = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        rig.shell
            .open_spec(PaneSpec::editor("t1", "src/a.ts").unwrap());
        let file = rig.shell.layout().tabs()[0].pane_ids()[1].clone();
        rig.shell
            .apply(vec![layout::Command::ClosePane { pane: file }]);
        rig.shell
            .apply(vec![layout::Command::ClosePane { pane: chat }]);
        assert!(rig.shell.reopen_closed(), "the file is reopened");
        assert!(!rig.shell.reopen_closed(), "only once, and not the chat");
        assert_eq!(rig.shell.layout().pane_count(), 1, "the file is back alone");
    }

    #[test]
    fn a_draft_opens_as_a_tab_of_the_active_space() {
        let mut rig = shell_rig();
        rig.space.select("p1");
        let draft = store::DraftThread {
            id: "draft-u1".into(),
            project: "p1".into(),
            repo: String::new(),
            workspace: "w".into(),
            parent: String::new(),
        };
        let params = crate::panes::kinds::core_panes::draft_params(&draft).unwrap();
        rig.shell.open_view(chat_type().unwrap(), params);
        let spaces: Vec<&str> = rig.shell.layout().tabs().iter().map(Tab::space).collect();
        assert_eq!(spaces, ["p1"], "the draft's tab is the active space's");
    }

    #[test]
    fn a_sidebar_row_hands_the_keyboard_to_a_chat_it_opens_but_not_to_one_already_open() {
        let cases = [("t2", true), ("t1", false)];
        for (chat, page_keys) in cases {
            let mut rig = shell_rig();
            synced_chat(&mut rig);
            rig.shell.keep_keyboard_in_shell();
            let row = (chat_type().unwrap(), chat_params(chat).unwrap());
            rig.shell.open_from_sidebar(row);
            resync(&mut rig);
            let pane = rig.shell.showing().and_then(Tab::focused).cloned().unwrap();
            let took = page_takes_keys(&mut rig, pane);
            assert_eq!(took, page_keys, "{chat}");
        }
    }
}
