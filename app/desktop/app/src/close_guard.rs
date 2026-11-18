use std::collections::BTreeSet;

use layout::{Command, Layout};
use protocol::PaneId;

use crate::dirty_panes::DirtyPanes;

pub(crate) fn closing_panes(
    layout: &Layout,
    command: &Command,
) -> Vec<PaneId> {
    match command {
        Command::ClosePane { pane } => vec![pane.clone()],
        Command::ClosePanes { panes } => panes.clone(),
        Command::CloseTab { tab } => layout
            .tab(*tab)
            .map_or_else(Vec::new, layout::Tab::pane_ids),
        _ => Vec::new(),
    }
}

pub(crate) fn dirty_among(
    layout: &Layout,
    commands: &[Command],
    flags: &DirtyPanes,
) -> Vec<PaneId> {
    let dirty: BTreeSet<PaneId> = commands
        .iter()
        .flat_map(|command| closing_panes(layout, command))
        .filter(|pane| flags.contains(pane))
        .collect();
    dirty.into_iter().collect()
}

#[cfg(test)]
mod tests {
    use layout::{PaneInstance, TabId};
    use protocol::PaneState;

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    fn open_chat(id: &str) -> Command {
        let instance = PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: chat_type().unwrap(),
            params: chat_params(id).unwrap(),
            state: PaneState::default(),
        };
        Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        }
    }

    fn layout_with(panes: &[&str]) -> Layout {
        let mut layout = Layout::default();
        let add = Command::AddTab {
            id: TabId(1),
            title: String::new(),
        };
        layout.apply(add).unwrap();
        for id in panes {
            layout.apply(open_chat(id)).unwrap();
        }
        layout
    }

    fn close(pane: &str) -> Command {
        Command::ClosePane {
            pane: PaneId::new(pane).unwrap(),
        }
    }

    fn close_many(panes: &[&str]) -> Command {
        Command::ClosePanes {
            panes: panes
                .iter()
                .map(|pane| PaneId::new(*pane).unwrap())
                .collect(),
        }
    }

    struct Case {
        name: &'static str,
        commands: Vec<Command>,
        expected: Vec<&'static str>,
    }

    #[test]
    fn only_dirty_panes_the_commands_close_are_named_once_each() {
        let layout = layout_with(&["a", "b", "c"]);
        let flags = DirtyPanes::default();
        flags.mark(PaneId::new("a").unwrap());
        flags.mark(PaneId::new("c").unwrap());
        let cases = [
            Case {
                name: "one dirty pane",
                commands: vec![close("a")],
                expected: vec!["a"],
            },
            Case {
                name: "a clean pane",
                commands: vec![close("b")],
                expected: vec![],
            },
            Case {
                name: "bulk close names each once",
                commands: vec![close("a"), close("a"), close("b")],
                expected: vec!["a"],
            },
            Case {
                name: "a bulk close names the dirty panes among its list",
                commands: vec![close_many(&["a", "b"])],
                expected: vec!["a"],
            },
            Case {
                name: "closing the tab names every dirty pane in it",
                commands: vec![Command::CloseTab { tab: TabId(1) }],
                expected: vec!["a", "c"],
            },
        ];
        for case in cases {
            let named: Vec<String> = dirty_among(&layout, &case.commands, &flags)
                .iter()
                .map(ToString::to_string)
                .collect();
            assert_eq!(named, case.expected, "{}", case.name);
        }
    }

    #[test]
    fn commands_that_close_nothing_name_nothing() {
        let layout = layout_with(&["a"]);
        let flags = DirtyPanes::default();
        flags.mark(PaneId::new("a").unwrap());
        let focus = Command::FocusPane {
            pane: PaneId::new("a").unwrap(),
        };
        let named = dirty_among(&layout, &[focus], &flags);
        assert!(named.is_empty(), "closes nothing: {named:?}");
    }
}
