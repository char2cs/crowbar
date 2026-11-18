//! Where the sidebar sends its commands.

use crate::error::Error;
use crate::shell::command::Command;

/// Implemented by the shell. A refusal comes back as [`Error::Refused`] so the view can keep an
/// inline input open with the reason.
pub trait CommandSink {
    /// Performs `command`.
    ///
    /// # Errors
    ///
    /// [`Error::Refused`] when the shell rejects the command; the sidebar keeps the input open and
    /// shows the reason.
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error>;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixtures::DemoData;
    use crate::shell::chats::RowId;

    #[test]
    fn a_sink_may_refuse_a_command_with_a_reason() {
        let data = DemoData::reference();
        let refused = data.dispatch(Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        });
        assert!(matches!(refused, Err(Error::Refused { .. })));
    }

    #[test]
    fn a_sink_accepts_a_valid_command() {
        let data = DemoData::reference();
        assert!(data.dispatch(Command::OpenRow(RowId::from("chat"))).is_ok());
    }
}
