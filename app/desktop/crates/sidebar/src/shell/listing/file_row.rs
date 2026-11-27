use crate::shell::listing::file_kind::FileKind;
use crate::shell::listing::git_status::GitStatus;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileRow {
    pub path: String,
    pub name: String,
    pub depth: u8,
    pub kind: FileKind,
    pub status: GitStatus,
}
