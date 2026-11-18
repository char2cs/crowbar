use layout::{Command, Layout};

use crate::close_guard::closing_panes;
use crate::pane_spec::PaneSpec;

const REMEMBERED: usize = 20;

#[derive(Debug, Default)]
pub(crate) struct ClosedPanes {
    specs: Vec<PaneSpec>,
}

impl ClosedPanes {
    pub(crate) fn closing(
        layout: &Layout,
        command: &Command,
    ) -> Vec<PaneSpec> {
        closing_panes(layout, command)
            .iter()
            .filter_map(|pane| layout.pane(pane))
            .map(PaneSpec::of)
            .collect()
    }

    pub(crate) fn remember(
        &mut self,
        closed: Vec<PaneSpec>,
    ) {
        self.specs.extend(closed);
        let excess = self.specs.len().saturating_sub(REMEMBERED);
        self.specs.drain(..excess);
    }

    pub(crate) fn take_last(&mut self) -> Option<PaneSpec> {
        self.specs.pop()
    }
}

#[cfg(test)]
mod tests {
    use layout::{PaneInstance, TabId};
    use protocol::{PaneId, PaneState};

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    fn chat_instance(id: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: chat_type().unwrap(),
            params: chat_params(id).unwrap(),
            state: PaneState::default(),
        }
    }

    fn open_pane(
        layout: &mut Layout,
        instance: PaneInstance,
    ) {
        let command = Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        };
        layout.apply(command).unwrap();
    }

    fn layout_with(panes: &[&str]) -> Layout {
        let mut layout = Layout::default();
        let add = Command::AddTab {
            id: TabId(1),
            title: String::new(),
        };
        layout.apply(add).unwrap();
        for id in panes {
            open_pane(&mut layout, chat_instance(id));
        }
        layout
    }

    type Case = (&'static str, fn() -> Command, &'static [&'static str]);

    const CASES: [Case; 4] = [
        (
            "a tab closes its panes",
            || Command::CloseTab { tab: TabId(1) },
            &["a", "b"],
        ),
        (
            "a pane closes itself",
            || Command::ClosePane {
                pane: PaneId::new("b").unwrap(),
            },
            &["b"],
        ),
        (
            "a bulk close names each of its panes",
            || Command::ClosePanes {
                panes: vec![PaneId::new("a").unwrap(), PaneId::new("b").unwrap()],
            },
            &["a", "b"],
        ),
        (
            "other commands close nothing",
            || Command::FocusPane {
                pane: PaneId::new("a").unwrap(),
            },
            &[],
        ),
    ];

    #[test]
    fn closing_names_the_panes_the_command_closes() {
        let layout = layout_with(&["a", "b"]);
        for (name, command, expected) in CASES {
            let closing = ClosedPanes::closing(&layout, &command());
            let threads: Vec<_> = closing
                .iter()
                .map(|spec| spec.params.get("threadId").unwrap())
                .collect();
            assert_eq!(threads, expected, "{name}");
        }
    }

    #[test]
    fn the_last_closed_comes_back_first_and_only_the_newest_are_kept() {
        let layout = layout_with(&["a", "b"]);
        let both = ClosedPanes::closing(&layout, &Command::CloseTab { tab: TabId(1) });
        let mut closed = ClosedPanes::default();
        closed.remember(both.clone());
        let last = closed.take_last().unwrap();
        assert_eq!(last.params.get("threadId"), Some("b"), "newest first");
        assert!(closed.take_last().is_some(), "then the other");
        assert!(closed.take_last().is_none(), "then nothing");
        for _ in 0..REMEMBERED {
            closed.remember(both.clone());
        }
        assert_eq!(closed.specs.len(), REMEMBERED, "capped");
    }
}
