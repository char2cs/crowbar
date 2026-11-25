use crate::shell::dialogs::pending_file::PendingFile;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum PromptEvent {
    Confirmed(PendingFile),
    Dismissed,
}
