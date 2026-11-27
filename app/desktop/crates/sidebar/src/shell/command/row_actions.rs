use crate::shell::chats::MoveTarget;
use crate::shell::chats::RowId;
use crate::shell::command::Command;

pub trait RowActions {
    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget>;
    fn serves(
        &self,
        _command: &Command,
    ) -> bool {
        true
    }
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
