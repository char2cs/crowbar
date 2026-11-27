use crate::shell::chats::Expansion;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileKind {
    File,
    Directory(Expansion),
}
