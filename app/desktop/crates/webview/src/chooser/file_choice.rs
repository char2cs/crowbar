use crate::chooser::choice_kind::ChoiceKind;
use crate::chooser::file_filter::FileFilter;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FileChoice {
    pub title: &'static str,
    pub kind: ChoiceKind,
    pub filter: Option<FileFilter>,
}
