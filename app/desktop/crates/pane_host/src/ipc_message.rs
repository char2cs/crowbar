use protocol::PaneId;

/// A message a page posted, tagged by the pane that owns the web view it came from.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IpcMessage {
    /// The pane that owned the sending web view at the time.
    pub pane: PaneId,
    /// The message text, a control frame in JSON.
    pub text: String,
}
