use protocol::{PaneTypeId, Params};

use crate::core_panes::{chat_params, chat_type};
use crate::error::Result;

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum SidebarAction {
    Open(PaneTypeId, Params),
    OpenSettings,
    OpenFile(String),
    ImportRepo,
    ImportBranches(String),
    Reveal(String),
    Unserved,
}

pub(crate) fn action_for(command: &sidebar::Command) -> Result<SidebarAction> {
    match command {
        sidebar::Command::OpenRow(row) => Ok(SidebarAction::Open(
            chat_type()?,
            chat_params(row.as_str())?,
        )),
        sidebar::Command::OpenSettings | sidebar::Command::OpenHostSettings(_) => {
            Ok(SidebarAction::OpenSettings)
        },
        sidebar::Command::OpenFile(path) => Ok(SidebarAction::OpenFile(path.clone())),
        sidebar::Command::ImportRepo => Ok(SidebarAction::ImportRepo),
        sidebar::Command::ImportBranches(row) => {
            Ok(SidebarAction::ImportBranches(row.as_str().to_owned()))
        },
        sidebar::Command::RevealFile { path } => Ok(SidebarAction::Reveal(path.clone())),
        _ => Ok(SidebarAction::Unserved),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Command, RowId};

    use super::*;

    #[test]
    fn sidebar_commands_become_the_actions_the_shell_carries_out() {
        let cases = [
            (
                Command::OpenRow(RowId::from("t1")),
                SidebarAction::Open(chat_type().unwrap(), chat_params("t1").unwrap()),
            ),
            (Command::OpenSettings, SidebarAction::OpenSettings),
            (
                Command::ImportBranches(RowId::from("r")),
                SidebarAction::ImportBranches("r".into()),
            ),
            (
                Command::RevealFile {
                    path: "src/a.rs".into(),
                },
                SidebarAction::Reveal("src/a.rs".into()),
            ),
        ];
        for (command, expected) in cases {
            assert_eq!(action_for(&command).unwrap(), expected, "{command:?}");
        }
    }
}
