//! A directory of a chat's workspace as the daemon just listed it.

use crate::live::held::FileEntry;

#[derive(Debug, Clone, Copy)]
pub(crate) struct ListedDirectory<'a> {
    pub(crate) chat: &'a str,
    pub(crate) directory: &'a str,
    pub(crate) entries: &'a [FileEntry],
}
