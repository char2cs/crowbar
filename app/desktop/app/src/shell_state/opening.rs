use layout::{Placement, TabId};
use protocol::{PaneTypeId, Params};

use super::ShellState;
use super::preview::keeping;
use crate::core_panes::{chat_params, chat_type};
use crate::pane_opener::{open_at, open_view_commands};
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(crate) fn open_view(
        &mut self,
        type_id: PaneTypeId,
        params: Params,
    ) {
        match open_view_commands(self.history.layout(), (type_id, params), &self.serials) {
            Ok(commands) => self.apply(commands),
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
        match spec.open_commands(self.history.layout(), &self.serials) {
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
    use crate::test_support::{open_chat, shell_rig};

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
        let active = rig.shell.layout().active_tab().map(layout::Tab::id);
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
    fn a_closed_pane_comes_back_once_and_nothing_closed_reopens_nothing() {
        let mut rig = shell_rig();
        assert!(!rig.shell.reopen_closed(), "nothing closed yet");
        open_chat(&mut rig.shell, "t1");
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        rig.shell.apply(vec![layout::Command::ClosePane { pane }]);
        assert!(rig.shell.reopen_closed(), "reopened");
        assert!(!rig.shell.reopen_closed(), "only once");
        assert_eq!(rig.shell.layout().pane_count(), 1, "back");
    }
}
