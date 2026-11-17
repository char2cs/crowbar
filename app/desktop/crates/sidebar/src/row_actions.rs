//! What a row may do: where it moves, which commands exist, which folders can leave.

use crate::command::Command;
use crate::move_target::MoveTarget;
use crate::row_id::RowId;

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
    use crate::demo_data::DemoData;

    #[test]
    fn nothing_is_refused_or_removable_unless_the_demo_says_so() {
        let data = DemoData::reference();
        assert!(data.serves(&Command::SelectProject("inv".into())));
        assert!(!data.can_remove_folder("src"));
        assert_eq!(data.move_targets(&RowId::from("chat")).len(), 0);
    }
}
