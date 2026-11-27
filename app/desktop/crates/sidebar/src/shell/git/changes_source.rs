use crate::shell::git::changes::Changes;
use crate::shell::git::merge_box::MergeBox;

pub trait ChangesSource {
    fn changes(&self) -> Changes;
    fn merge_box(&self) -> Option<MergeBox> {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::DemoShell;

    #[test]
    fn the_demo_workspace_has_changes_and_no_merge_box() {
        let data = DemoShell::reference();
        assert_eq!(data.changes().files.len(), 1);
        assert_eq!(data.merge_box(), None);
    }
}
