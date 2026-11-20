use protocol::{PaneTypeId, Params};

use crate::core_panes::{chat_params, chat_type};
use crate::error::Result;

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum SidebarAction {
    Open(PaneTypeId, Params),
    OpenSettings,
    OpenHosts,
    OpenFile(String),
    OpenReview(String),
    ImportRepo,
    ChooseSpaceFolder(String),
    ImportBranches(String),
    Reveal(String),
    SetFileTreeFlag { key: &'static str, on: bool },
    Unserved,
}

pub(crate) fn action_for(command: &sidebar::Command) -> Result<SidebarAction> {
    match command {
        sidebar::Command::OpenRow(row) => Ok(SidebarAction::Open(
            chat_type()?,
            chat_params(row.as_str())?,
        )),
        sidebar::Command::OpenSettings => Ok(SidebarAction::OpenSettings),
        sidebar::Command::OpenHostSettings(_) => Ok(SidebarAction::OpenHosts),
        sidebar::Command::OpenFile(path) => Ok(SidebarAction::OpenFile(path.clone())),
        sidebar::Command::OpenReview { path } => Ok(SidebarAction::OpenReview(path.clone())),
        sidebar::Command::ImportRepo => Ok(SidebarAction::ImportRepo),
        sidebar::Command::ChooseSpaceFolder(host) => {
            Ok(SidebarAction::ChooseSpaceFolder(host.as_str().to_owned()))
        },
        sidebar::Command::ImportBranches(row) => {
            Ok(SidebarAction::ImportBranches(row.as_str().to_owned()))
        },
        sidebar::Command::RevealFile { path } => Ok(SidebarAction::Reveal(path.clone())),
        sidebar::Command::SetFileTreeFlag { flag, on } => Ok(SidebarAction::SetFileTreeFlag {
            key: flag.setting_key(),
            on: *on,
        }),
        _ => Ok(SidebarAction::Unserved),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Command, RowId};

    use super::*;

    fn review_case() -> (Command, SidebarAction) {
        let path = String::from("src/a.rs");
        let action = SidebarAction::OpenReview(path.clone());
        (Command::OpenReview { path }, action)
    }

    fn cases() -> Vec<(Command, SidebarAction)> {
        vec![
            (
                Command::OpenRow(RowId::from("t1")),
                SidebarAction::Open(chat_type().unwrap(), chat_params("t1").unwrap()),
            ),
            (Command::OpenSettings, SidebarAction::OpenSettings),
            review_case(),
            (
                Command::OpenHostSettings(sidebar::HostId::from("box")),
                SidebarAction::OpenHosts,
            ),
            (
                Command::ChooseSpaceFolder(sidebar::HostId::from("box")),
                SidebarAction::ChooseSpaceFolder("box".into()),
            ),
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
            (
                Command::SetFileTreeFlag {
                    flag: sidebar::FileTreeFlag::Gitignored,
                    on: false,
                },
                SidebarAction::SetFileTreeFlag {
                    key: "showGitignoredFilesInFileTree",
                    on: false,
                },
            ),
        ]
    }

    #[test]
    fn sidebar_commands_become_the_actions_the_shell_carries_out() {
        for (command, expected) in cases() {
            assert_eq!(action_for(&command).unwrap(), expected, "{command:?}");
        }
    }
}
