use crate::error::Result;
use crate::ide::command::close_panes;
use crate::ide::command::group_commands;
use crate::ide::command::pin_commands;
use crate::ide::command::preview_commands;
use crate::ide::command::view_commands;
use crate::layout::Layout;
use crate::layout::command::Command;
use crate::layout::command::pane_commands;
use crate::layout::command::tab_commands;
use crate::layout::tab::pane_instance::PaneInstance;

impl Layout {
    pub(crate) fn execute(
        &mut self,
        command: Command,
    ) -> Result<Vec<PaneInstance>> {
        match command {
            Command::AddTab { id, title, space } => {
                tab_commands::add(self, id, title, space).map(none_removed)
            },
            Command::CloseTab { tab } => tab_commands::close(self, tab),
            Command::ActivateTab { tab } => tab_commands::activate(self, tab).map(none_removed),
            Command::MoveTab { tab, to_index } => {
                tab_commands::move_to(self, tab, to_index).map(none_removed)
            },
            Command::RenameTab { tab, title } => {
                tab_commands::rename(self, tab, title).map(none_removed)
            },
            Command::ResizeSplit {
                tab,
                path,
                bounds,
                first_extent,
            } => self
                .tab_mut(tab)
                .and_then(|t| t.resize(&path, bounds, first_extent))
                .map(none_removed),
            Command::ResetSplit { tab, path } => {
                tab_commands::equalize(self, tab, &path).map(none_removed)
            },
            marking @ (Command::PreviewPane { .. } | Command::KeepPane { .. }) => {
                preview_commands::execute(self, marking).map(none_removed)
            },
            Command::AdoptSpace { tab, space } => {
                tab_commands::adopt(self, tab, space).map(none_removed)
            },
            Command::SetEditorView { pane, view } => {
                view_commands::set_editor_view(self, &pane, view).map(none_removed)
            },
            pane_command => self.execute_pane(pane_command),
        }
    }

    fn execute_pane(
        &mut self,
        command: Command,
    ) -> Result<Vec<PaneInstance>> {
        match command {
            Command::OpenPane {
                tab,
                instance,
                placement,
            } => pane_commands::open(self, tab, instance, placement).map(none_removed),
            Command::OpenPaneInGroup {
                instance,
                onto,
                index,
            } => group_commands::open(self, instance, (&onto, index)).map(none_removed),
            Command::MoveToGroup { pane, onto, index } => {
                group_commands::join(self, &pane, (&onto, index)).map(none_removed)
            },
            Command::ClosePane { pane } => pane_commands::close(self, &pane),
            Command::ClosePanes { panes } => close_panes::close_many(self, &panes),
            Command::PinPane { pane } => pin_commands::pin(self, &pane).map(none_removed),
            Command::UnpinPane { pane } => pin_commands::unpin(self, &pane).map(none_removed),
            Command::FocusPane { pane } => pane_commands::focus(self, &pane).map(none_removed),
            Command::MovePane { pane, placement } => {
                pane_commands::relocate(self, &pane, &placement).map(none_removed)
            },
            Command::MovePaneToTab {
                pane,
                to,
                placement,
            } => pane_commands::move_to_tab(self, &pane, to, placement).map(none_removed),
            Command::DetachPane {
                pane,
                new_tab,
                title,
            } => pane_commands::detach(self, &pane, new_tab, title).map(none_removed),
            Command::SwapPanes { a, b } => pane_commands::swap(self, &a, &b).map(none_removed),
            content @ (Command::SetPaneState { .. } | Command::SetPaneParams { .. }) => {
                pane_commands::set_content(self, content).map(none_removed)
            },
            _ => Ok(Vec::new()),
        }
    }
}

fn none_removed(_: ()) -> Vec<PaneInstance> {
    Vec::new()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tab_id::TabId;

    #[test]
    fn executing_add_then_close_returns_the_closed_tabs_panes() {
        let mut layout = Layout::default();
        let tab = TabId(1);
        layout
            .execute(Command::AddTab {
                id: tab,
                title: "t".into(),
                space: crate::space_id::SpaceId::default(),
            })
            .unwrap();
        assert!(layout.tab(tab).is_some(), "tab added");
        let removed = layout.execute(Command::CloseTab { tab }).unwrap();
        assert!(removed.is_empty(), "an empty tab holds no panes");
        assert!(layout.tab(tab).is_none(), "tab closed");
    }
}
