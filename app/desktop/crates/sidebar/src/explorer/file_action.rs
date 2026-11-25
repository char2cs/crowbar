//! What choosing an entry of a file row's menu does.

use crate::explorer::CopyNote;
use crate::shell::Command;
use crate::shell::Intent;
use crate::shell::NewEntry;

/// The effect of one file-menu entry.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FileAction {
    /// Carry out the intent.
    Do(Intent),
    /// Put the path on the system clipboard, and note on the row which one it was.
    CopyText(String, CopyNote),
}

fn create_probe(entry: NewEntry) -> Command {
    let path = String::new();
    match entry {
        NewEntry::File => Command::CreateFile { path },
        NewEntry::Folder => Command::CreateDirectory { path },
    }
}

fn rename_probe(path: &str) -> Command {
    Command::RenameFile {
        path: path.to_owned(),
        new_path: String::new(),
    }
}

fn delete_probe(path: &str) -> Command {
    Command::DeleteFile {
        path: path.to_owned(),
    }
}

impl FileAction {
    /// The command this entry sends or starts, if any, so the menu can ask the shell whether it
    /// serves it. Starting a create or rename names its command with an empty new path.
    pub fn command(&self) -> Option<Command> {
        let Self::Do(intent) = self else {
            return None;
        };
        match intent {
            Intent::Send(command) => Some(command.clone()),
            Intent::StartFileCreate { entry, .. } => Some(create_probe(*entry)),
            Intent::StartFileRename(path) => Some(rename_probe(path)),
            Intent::RequestFileDelete { path, .. } => Some(delete_probe(path)),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_entry_names_the_command_it_will_send() {
        let delete = FileAction::Do(Intent::RequestFileDelete {
            path: "a".into(),
            is_dir: false,
        });
        assert_eq!(
            delete.command(),
            Some(Command::DeleteFile { path: "a".into() })
        );
        assert_eq!(
            FileAction::CopyText("a".into(), CopyNote::Path).command(),
            None
        );
        assert_eq!(
            FileAction::Do(Intent::ShowProperties("a".into())).command(),
            None
        );
    }
}
