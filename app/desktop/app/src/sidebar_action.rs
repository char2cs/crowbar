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
        _ => Ok(SidebarAction::Unserved),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Command, RowId};

    use super::*;

    #[test]
    fn opening_a_row_opens_its_chat() {
        let action = action_for(&Command::OpenRow(RowId::from("t1"))).unwrap();
        assert_eq!(
            action,
            SidebarAction::Open(chat_type().unwrap(), chat_params("t1").unwrap()),
            "chat of the thread"
        );
    }

    #[test]
    fn settings_open_the_modal_and_the_branch_importer_names_its_row() {
        assert_eq!(
            action_for(&Command::OpenSettings).unwrap(),
            SidebarAction::OpenSettings,
            "settings"
        );
        assert_eq!(
            action_for(&Command::ImportBranches(RowId::from("r"))).unwrap(),
            SidebarAction::ImportBranches("r".into()),
            "import branches"
        );
    }
}
