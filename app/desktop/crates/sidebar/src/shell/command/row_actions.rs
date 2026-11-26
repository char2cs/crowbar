//! What a row may do: where it moves, which commands exist, which folders can leave.

use crate::shell::chats::MoveTarget;
use crate::shell::chats::RowId;
use crate::shell::command::Command;

/// What a row may do: where it moves, which commands exist, which folders can leave.
pub trait RowActions {
    /// The folders and branches `row` can move into.
    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget>;
    /// Whether the shell carries out `command`; the sidebar hides the entries whose command it
    /// does not.
    fn serves(
        &self,
        _command: &Command,
    ) -> bool {
        true
    }
    /// Whether `path` is one of several folders of the workspace, which offers Remove From
    /// Workspace.
    fn can_remove_folder(
        &self,
        _path: &str,
    ) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::DemoShell;

    #[test]
    fn nothing_is_refused_or_removable_unless_the_demo_says_so() {
        let data = DemoShell::reference();
        assert!(data.serves(&Command::SelectProject("inv".into())));
        assert!(!data.can_remove_folder("src"));
        assert_eq!(data.move_targets(&RowId::from("chat")).len(), 0);
    }
}
