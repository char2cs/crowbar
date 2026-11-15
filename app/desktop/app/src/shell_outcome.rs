use protocol::{CommandId, PaneId};

/// What the shell must still do after the pane host reported events.
#[derive(Debug, Default, PartialEq, Eq)]
pub(crate) struct Outcome {
    pub(crate) titles_changed: bool,
    pub(crate) hover: Vec<(PaneId, bool)>,
    pub(crate) commands: Vec<CommandId>,
}
