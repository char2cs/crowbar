use protocol::{CommandId, PaneId};

#[derive(Debug, Default, PartialEq, Eq)]
pub(crate) struct Outcome {
    pub(crate) titles_changed: bool,
    pub(crate) hover: Vec<(PaneId, bool)>,
    pub(crate) commands: Vec<CommandId>,
    pub(crate) pages: Vec<(PaneId, String)>,
    pub(crate) settings: Vec<(protocol::SettingKey, serde_json::Value)>,
}
