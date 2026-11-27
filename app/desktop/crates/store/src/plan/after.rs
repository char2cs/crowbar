use crate::plan::removal::Taken;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum After {
    Nothing,
    OpenChat(String),
    OpenDraft(crate::live::DraftThread),
    SelectProject(String),
    ReloadChanges,
    NameFolder,
    Place(String),
    SettleName(sidebar::RowId),
    AnnounceTaken(Taken),
}
