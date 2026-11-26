//! The title develop's failure toast gives each sidebar command, over the daemon's reason.

use sidebar::Command;

use crate::live::State;
use crate::rows::target::{Kind, resolve};

pub(crate) fn failure_title(
    state: &State,
    command: &Command,
) -> Option<String> {
    let title = match command {
        Command::Rename { row, .. } => rename_title(state, row.as_str())?,
        Command::Lock(_) | Command::Unlock(_) => "Failed to update lock".to_owned(),
        Command::CreateFolder(_) => "Failed to create folder".to_owned(),
        Command::CreateSpace { name, .. } | Command::CreateSpaceWithIcon { name, .. } => {
            format!("Add \u{201c}{name}\u{201d} failed")
        },
        Command::SetSpaceIcon { icon, .. } | Command::SetRepoIcon { icon, .. } => {
            icon_title(icon).to_owned()
        },
        Command::UseGithubAvatar(_) => "Could not fetch the GitHub avatar".to_owned(),
        Command::Fork { name, .. } => format!("Create {name} failed"),
        Command::NewThread(_)
        | Command::NewChat(_)
        | Command::NewTerminalThread(_)
        | Command::Thread(_)
        | Command::TerminalThread(_) => "New thread failed".to_owned(),
        Command::Dismiss(_) => "Dismiss".to_owned(),
        Command::MoveTo { .. } | Command::Drop { .. } => "Move".to_owned(),
        Command::CancelMove(_) => "Cancel move".to_owned(),
        _ => return None,
    };
    Some(title)
}

fn icon_title(icon: &sidebar::SpaceIcon) -> &'static str {
    match icon {
        sidebar::SpaceIcon::Emoji(_) => "Failed to set emoji",
        sidebar::SpaceIcon::Default => "Failed to reset icon",
        sidebar::SpaceIcon::Image(_) => "Failed to set icon",
    }
}

fn rename_title(
    state: &State,
    row: &str,
) -> Option<String> {
    let noun = match resolve(state, row)?.kind {
        Kind::Folder => "folder",
        Kind::RepoHeader { .. } => "repository",
        Kind::Branch { .. } => "branch",
        Kind::Chat { .. } => "chat",
    };
    Some(format!("Failed to rename {noun}"))
}

#[cfg(test)]
mod tests {
    use sidebar::{ProjectId, RowId};

    use super::*;
    use crate::test_support::state;

    #[test]
    fn each_command_fails_under_develops_title() {
        let (state, _) = state();
        let cases = [
            (
                Command::Lock(RowId::from("b")),
                Some("Failed to update lock"),
            ),
            (Command::CreateFolder(None), Some("Failed to create folder")),
            (
                Command::NewThread(ProjectId::from("p")),
                Some("New thread failed"),
            ),
            (Command::CancelMove(RowId::from("b")), Some("Cancel move")),
            (Command::OpenSettings, None),
        ];
        for (command, title) in cases {
            let got = failure_title(&state, &command);
            assert_eq!(got.as_deref(), title, "{command:?}");
        }
    }

    #[test]
    fn a_rename_is_titled_by_the_kind_of_row() {
        let (state, _) = state();
        let rename = |row: &str| Command::Rename {
            row: RowId::from(row),
            name: "x".into(),
        };
        let cases = [
            ("f", Some("Failed to rename folder")),
            ("rdo", Some("Failed to rename repository")),
            ("t", Some("Failed to rename chat")),
            ("ghost", None),
        ];
        for (row, title) in cases {
            let got = failure_title(&state, &rename(row));
            assert_eq!(got.as_deref(), title, "{row}");
        }
    }
}
