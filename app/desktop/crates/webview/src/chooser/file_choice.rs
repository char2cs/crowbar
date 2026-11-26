use crate::chooser::choice_kind::ChoiceKind;
use crate::chooser::file_filter::FileFilter;

/// One question to the desktop's file chooser.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FileChoice {
    /// The chooser's title.
    pub title: &'static str,
    /// Whether one file, several files or one folder is chosen.
    pub kind: ChoiceKind,
    /// The files listed; every file when `None`.
    pub filter: Option<FileFilter>,
}
