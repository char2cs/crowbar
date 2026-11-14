//! The menu behind "More…" at the foot of a space.

use ui::IconName;

use crate::command::Command;
use crate::intent::Intent;
use crate::menu_entry::MenuEntry;
use crate::project_id::ProjectId;

/// Import a repo, create a folder, delete the space; the destructive entry has no separator.
pub fn foot_menu(project: &ProjectId) -> Vec<MenuEntry> {
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
#[path = "foot_menu_tests.rs"]
mod tests;
