use protocol::{CommandId, PaneId};

#[derive(Debug, Default, PartialEq, Eq)]
pub(crate) struct Outcome {
    pub(crate) titles_changed: bool,
    pub(crate) dirty_changed: bool,
    pub(crate) hover: Vec<(PaneId, bool)>,
    pub(crate) focused: Vec<PaneId>,
    pub(crate) dropped: Vec<protocol::FsPath>,
    pub(crate) commands: Vec<CommandId>,
    pub(crate) pages: Vec<(PaneId, String)>,
    pub(crate) loaded: Vec<PaneId>,
    pub(crate) picks: Vec<(PaneId, browser_nav::PageUpdate)>,
    pub(crate) settings: Vec<(protocol::SettingKey, serde_json::Value)>,
}
