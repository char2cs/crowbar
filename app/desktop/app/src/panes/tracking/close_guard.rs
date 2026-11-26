use std::collections::BTreeSet;

use layout::{Command, Layout};
use protocol::PaneId;

use crate::panes::tracking::dirty_panes::DirtyPanes;

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
    use layout::TabId;

    use super::*;
    use crate::panes::kinds::core_panes::CHAT;
    use crate::test_support::{chat_pane, one_tab};

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

    #[test]
    fn only_dirty_panes_the_commands_close_are_named_once_each() {
        let layout = one_tab(["a", "b", "c"].map(|id| chat_pane((id, CHAT), id)));
        let flags = DirtyPanes::default();
        flags.mark(PaneId::new("a").unwrap());
        flags.mark(PaneId::new("c").unwrap());
        let focus = Command::FocusPane {
            pane: PaneId::new("a").unwrap(),
        };
        let cases = [
            ("one dirty pane", vec![close("a")], vec!["a"]),
            ("a clean pane", vec![close("b")], vec![]),
            (
                "bulk close names each once",
                vec![close("a"), close("a"), close("b")],
                vec!["a"],
            ),
            (
                "a bulk close names the dirty panes among its list",
                vec![close_many(&["a", "b"])],
                vec!["a"],
            ),
            (
                "closing the tab names every dirty pane in it",
                vec![Command::CloseTab { tab: TabId(1) }],
                vec!["a", "c"],
            ),
            ("a command that closes nothing", vec![focus], vec![]),
        ];
        for (name, commands, expected) in cases {
            let named: Vec<String> = dirty_among(&layout, &commands, &flags)
                .iter()
                .map(ToString::to_string)
                .collect();
            assert_eq!(named, expected, "{name}");
        }
    }
}
