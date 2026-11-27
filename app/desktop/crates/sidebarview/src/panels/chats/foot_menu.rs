use sidebar::{Command, Intent, ProjectId};

use crate::overlays::MenuEntry;

pub(crate) fn foot_menu(project: &ProjectId) -> Vec<MenuEntry> {
    vec![
        MenuEntry::action("Import a repo", Intent::Send(Command::ImportRepo)),
        MenuEntry::action(
            "Create a new folder",
            Intent::Send(Command::CreateFolder(None)),
        ),
        MenuEntry::action(
            "Delete space",
            Intent::RequestProjectRemoval(project.clone()),
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn three_entries_with_delete_space_last() {
        let entries = foot_menu(&ProjectId::from("p"));
        assert_eq!(entries.len(), 3, "import, folder, delete");
        assert!(
            matches!(&entries[2], MenuEntry::Action { label, .. } if label == "Delete space"),
            "delete space is last"
        );
    }
}
