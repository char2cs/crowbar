use protocol::{PaneTypeId, Params};

use crate::core_panes::{chat_params, chat_type};
use crate::error::Result;

/// What a sidebar command means for the shell.
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum SidebarAction {
    Open(PaneTypeId, Params),
    OpenSettings,
    Unserved,
}

/// Opening a thread opens its chat; everything that needs the daemon's project and thread
/// data is not served yet.
pub(crate) fn action_for(command: &sidebar::Command) -> Result<SidebarAction> {
    match command {
        sidebar::Command::Thread(row) | sidebar::Command::OpenRow(row) => Ok(SidebarAction::Open(
            chat_type()?,
            chat_params(row.as_str())?,
        )),
        sidebar::Command::OpenSettings => Ok(SidebarAction::OpenSettings),
        _ => Ok(SidebarAction::Unserved),
    }
}

#[cfg(test)]
#[path = "sidebar_action_tests.rs"]
mod tests;
