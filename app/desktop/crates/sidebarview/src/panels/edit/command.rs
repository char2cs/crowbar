//! The command a confirmed inline edit sends.

use sidebar::{Command, Editing, NewEntry, PathText, RowId};

fn creation(
    path: String,
    entry: NewEntry,
) -> Command {
    match entry {
        NewEntry::File => Command::CreateFile { path },
        NewEntry::Folder => Command::CreateDirectory { path },
    }
}

pub(super) fn command_for(
    editing: &Editing,
    row: &RowId,
    name: String,
) -> Command {
    match editing {
        Editing::Branching(target) => Command::MakeBranch {
            row: target.clone(),
            name,
        },
        Editing::Drafting(parent) => Command::Fork {
            parent: parent.clone(),
            name,
        },
        Editing::RenamingProject(project) => Command::RenameProject {
            project: project.clone(),
            name,
        },
        Editing::RenamingFile(path) => Command::RenameFile {
            path: path.clone(),
            new_path: PathText::new(PathText::new(path).parent()).join(&name),
        },
        Editing::Creating { dir, entry } => creation(PathText::new(dir).join(&name), *entry),
        _ => Command::Rename {
            row: row.clone(),
            name,
        },
    }
}

#[cfg(test)]
mod tests {
    use sidebar::ProjectId;

    use super::*;

    #[test]
    fn what_is_being_edited_decides_the_command() {
        let row = RowId::from("row");
        let space = ProjectId::from("space");
        let renaming = Editing::RenamingProject(space.clone());
        let sent = command_for(&renaming, &row, "Atlas".into());
        let renamed = Command::RenameProject {
            project: space,
            name: "Atlas".into(),
        };
        assert_eq!(sent, renamed);
        let moved = Editing::RenamingFile("src/a.rs".into());
        let sent = command_for(&moved, &row, "b.rs".into());
        let beside = Command::RenameFile {
            path: "src/a.rs".into(),
            new_path: "src/b.rs".into(),
        };
        assert_eq!(sent, beside);
    }
}
