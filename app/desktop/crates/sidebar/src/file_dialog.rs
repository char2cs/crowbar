//! The dialog a file row's menu can open.

use crate::file_properties::FileProperties;
use crate::sidebar::base;

/// A dialog opened from a file row's menu.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FileDialog {
    /// Asks before deleting a file or folder.
    Delete {
        /// The path of the file or folder to delete.
        path: String,
        /// Whether the path is a folder, deleted with all its contents.
        is_dir: bool,
    },
    /// Shows the Properties of one file.
    Properties(FileProperties),
}

impl FileDialog {
    /// The dialog's title, naming whether a file or a folder is deleted.
    pub fn title(&self) -> &'static str {
        match self {
            Self::Delete { is_dir: true, .. } => "Delete Folder",
            Self::Delete { is_dir: false, .. } => "Delete File",
            Self::Properties(_) => "Properties",
        }
    }

    /// The question a Delete dialog asks, naming the target; `None` for Properties.
    pub fn question(&self) -> Option<String> {
        let Self::Delete { path, is_dir } = self else {
            return None;
        };
        let name = base(path);
        if *is_dir {
            return Some(format!(
                "Are you sure you want to delete the folder \"{name}\" and all its contents? This action cannot be undone."
            ));
        }
        Some(format!(
            "Are you sure you want to delete the file \"{name}\"? This action cannot be undone."
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn deleting_names_the_target_and_properties_asks_nothing() {
        let file = FileDialog::Delete {
            path: "src/a.rs".into(),
            is_dir: false,
        };
        let folder = FileDialog::Delete {
            path: "src".into(),
            is_dir: true,
        };
        assert_eq!(file.title(), "Delete File");
        assert_eq!(folder.title(), "Delete Folder");
        assert_eq!(
            file.question().as_deref(),
            Some(
                "Are you sure you want to delete the file \"a.rs\"? This action cannot be undone."
            )
        );
        assert!(
            folder
                .question()
                .is_some_and(|text| text.contains("\"src\" and all its contents"))
        );
        let properties = FileDialog::Properties(FileProperties::of("a.rs"));
        assert_eq!(properties.title(), "Properties");
        assert_eq!(properties.question(), None);
    }
}
