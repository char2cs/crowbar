use protocol::PaneId;

use crate::command::Command;
use crate::error::Result;
use crate::layout_history::LayoutHistory;

impl LayoutHistory {
    /// Takes back the open of `pane`, as if it never happened, when that open is the last
    /// structural change: the layout returns to the arrangement before it, with its focus and
    /// the tab its space showed, and no undo step is left for it. A pane opened earlier closes as any other.
    ///
    /// # Errors
    /// The [`Error`](crate::Error) that refused closing `pane`.
    pub fn withdraw(
        &mut self,
        pane: &PaneId,
    ) -> Result<()> {
        let close = Command::ClosePane { pane: pane.clone() };
        let opened_last = self
            .undo
            .back()
            .is_some_and(|step| !step.arrangement.pane_ids().contains(pane));
        let Some(step) = self.undo.pop_back().filter(|_| opened_last) else {
            return self.apply(close);
        };
        if let Err(step) = self.current.swap_arrangement(step) {
            self.undo.push_back(step);
            return self.apply(close);
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use crate::command::Command;
    use crate::layout_history::LayoutHistory;
    use crate::pane_instance::PaneInstance;
    use crate::tab_id::TabId;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn open(name: &str) -> Command {
        let instance = PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new("core.editor").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        };
        Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        }
    }

    fn focus(name: &str) -> Command {
        Command::FocusPane { pane: id(name) }
    }

    fn add() -> Command {
        Command::AddTab {
            id: TabId(1),
            title: "t1".to_owned(),
            space: crate::SpaceId::default(),
        }
    }

    fn history_of(after: Vec<Command>) -> LayoutHistory {
        let mut history = LayoutHistory::default();
        let before = [add(), open("a"), open("b"), focus("a")];
        for command in before.into_iter().chain(after) {
            history.apply(command).unwrap();
        }
        history
    }

    #[test]
    fn withdrawing_returns_to_the_focus_before_the_open_or_closes_an_older_pane() {
        let cases = [
            ("the last open", vec![open("c")], "a", false),
            ("an older open", vec![open("c"), open("d")], "d", true),
        ];
        for (name, after, focused, undo_brings_it_back) in cases {
            let mut history = history_of(after);
            history.withdraw(&id("c")).unwrap();
            let tab = history.layout().tab(TabId(1)).unwrap();
            assert_eq!(tab.focused(), Some(&id(focused)), "{name}: focus");
            assert!(history.layout().pane(&id("c")).is_none(), "{name}: gone");
            history.undo();
            let back = history.layout().pane(&id("c")).is_some();
            assert_eq!(back, undo_brings_it_back, "{name}: undo");
        }
    }
}
