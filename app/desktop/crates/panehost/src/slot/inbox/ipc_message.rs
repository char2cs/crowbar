use protocol::PaneId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IpcMessage {
    pub pane: PaneId,
    pub text: String,
}
