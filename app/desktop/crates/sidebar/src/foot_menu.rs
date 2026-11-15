//! The menu behind "More…" at the foot of a space.

use ui::IconName;

use crate::command::Command;
use crate::intent::Intent;
use crate::menu_entry::MenuEntry;
use crate::project_id::ProjectId;

pub(crate) fn foot_menu(project: &ProjectId) -> Vec<MenuEntry> {
    vec![
        MenuEntry::action(
            "Import a repo",
            IconName::FolderOpen,
            Intent::Send(Command::ImportRepo),
        ),
        MenuEntry::action(
            "Create a new folder",
            IconName::Folder,
            Intent::Send(Command::CreateFolder(None)),
        ),
        MenuEntry::destructive(
            "Delete space",
            IconName::Trash,
            Intent::RequestProjectRemoval(project.clone()),
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tone::Tone;

    #[test]
    fn three_entries_with_the_destructive_one_last() {
        let entries = foot_menu(&ProjectId::from("p"));
        assert_eq!(entries.len(), 3, "import, folder, delete");
        assert!(
            matches!(&entries[2], MenuEntry::Action { label, tone: Tone::Destructive, .. } if label == "Delete space"),
            "delete space is destructive and last"
        );
    }
}
