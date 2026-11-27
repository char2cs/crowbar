pub(super) mod file_properties;

use crate::explorer::file_dialog::file_properties::FileProperties;
use crate::shell::PathText;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FileDialog {
    Delete { path: String, is_dir: bool },
    Properties(FileProperties),
}

impl FileDialog {
    pub fn title(&self) -> &'static str {
        match self {
            Self::Delete { is_dir: true, .. } => "Delete Folder",
            Self::Delete { is_dir: false, .. } => "Delete File",
            Self::Properties(_) => "Properties",
        }
    }

    pub fn question(&self) -> Option<String> {
        let Self::Delete { path, is_dir } = self else {
            return None;
        };
        let name = PathText::new(path).base();
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
