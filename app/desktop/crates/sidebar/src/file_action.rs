//! What choosing an entry of a file row's menu does.

use crate::command::Command;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum FileAction {
    Send(Command),
    CopyPath(String),
}
