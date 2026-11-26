use panehost::Keyboard;
use protocol::PaneId;

use crate::shell::shell_state::ShellState;

impl ShellState {
    pub(crate) fn withdraw_open(
        &mut self,
        pane: &PaneId,
    ) {
        if let Err(error) = self.history.withdraw(pane) {
            tracing::warn!(%error, "the layout refused to take back an open");
            return;
        }
        self.keyboard = Keyboard::Pane;
        self.settle();
    }

    pub(crate) fn settle(&mut self) {
        let layout = self.history.layout();
        self.titles.retain(|pane| layout.pane(pane).is_some());
        self.dirty.retain(|pane| layout.pane(pane).is_some());
        self.browsers.follow(self.history.layout());
        self.terminals.follow(self.history.layout());
        self.terminals.name_into(&self.titles);
        self.writer.submit(self.history.layout().clone());
    }
}

#[cfg(test)]
mod tests {
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::test_support::{open_chat, shell_rig};

    #[test]
    fn a_withdrawn_open_gives_focus_back_and_takes_its_title() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        let focused = |shell: &crate::shell::shell_state::ShellState| {
            let tab = shell.showing().unwrap();
            tab.focused().cloned().unwrap()
        };
        rig.shell
            .open_spec(PaneSpec::editor("t1", "src/a.ts").unwrap());
        let before = focused(&rig.shell);
        rig.shell
            .open_spec(PaneSpec::editor("t1", "src/b.ts").unwrap());
        let pane = before.clone();
        rig.shell.apply(vec![layout::Command::FocusPane { pane }]);
        rig.shell
            .open_spec(PaneSpec::editor("t1", "src/gone.ts").unwrap());
        let gone = focused(&rig.shell);
        rig.shell.withdraw_open(&gone);
        assert_eq!(focused(&rig.shell), before, "focus is back");
        assert!(
            rig.shell.titles().get(&gone).is_none(),
            "its title went with it"
        );
    }
}
